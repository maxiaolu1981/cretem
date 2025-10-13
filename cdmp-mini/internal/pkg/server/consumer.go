// internal/pkg/server/consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/usercache"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type UserConsumer struct {
	reader     *kafka.Reader
	db         *gorm.DB
	redis      *storage.RedisCluster
	producer   *UserProducer
	topic      string
	groupID    string
	instanceID int // 新增：实例ID
	opts       *options.KafkaOptions
	// 移除本地保护状态，全部走redis全局key
	// 主控选举相关
	isMaster bool
}

func NewUserConsumer(opts *options.KafkaOptions, topic, groupID string, db *gorm.DB, redis *storage.RedisCluster) *UserConsumer {
	consumer := &UserConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: opts.Brokers,
			Topic:   topic,
			GroupID: groupID,

			// 优化配置
			MinBytes: 1 * 1024 * 1024, // 降低最小字节数，立即消费
			//MinBytes:      10e3,
			MaxBytes:      10e6,                   // 10MB
			MaxWait:       time.Millisecond * 100, // 增加到100ms
			QueueCapacity: 100,                    // 降低队列容量，避免消息堆积在内存

			CommitInterval: 0,
			StartOffset:    kafka.FirstOffset,

			// 添加重试配置
			MaxAttempts:    opts.MaxRetries,
			ReadBackoffMin: time.Millisecond * 100,
			ReadBackoffMax: time.Millisecond * 1000,
		}),
		db:      db,
		redis:   redis,
		topic:   topic,
		groupID: groupID,
		opts:    opts,
		// 新增：实例ID赋值
		instanceID: parseInstanceID(opts.InstanceID),
	}
	//go consumer.startLagMonitor(context.Background())
	return consumer

}

// parseInstanceID 支持 string->int 转换，若失败则用 hash 兜底
func parseInstanceID(idStr string) int {
	if idStr == "" {
		return 0
	}
	if n, err := strconv.Atoi(idStr); err == nil {
		return n
	}
	// fallback: hash string
	sum := 0
	for _, c := range idStr {
		sum += int(c)
	}
	return sum & 0x7FFFFFFF // 保证正数
}

// 消费
func (c *UserConsumer) StartConsuming(ctx context.Context, workerCount int, ready *sync.WaitGroup) {
	log.Infof("[Consumer] StartConsuming: topic=%s, groupID=%s, workerCount=%d", c.topic, c.groupID, workerCount)
	// job 用于在 fetcher 与 worker 之间传递消息，并携带一个 done 通道用于返回处理结果
	type job struct {
		msg      kafka.Message
		done     chan error
		workerID int
	}

	jobs := make(chan *job, 2048) // 提升通道容量，支持高并发
	readyOnce := sync.Once{}
	signalReady := func() {
		if ready != nil {
			readyOnce.Do(func() {
				ready.Done()
			})
		}
	}
	defer signalReady()

	// 启动 worker 池，只负责处理业务，不直接调用 FetchMessage/CommitMessages
	var workerWg sync.WaitGroup
	// worker数量与分区数动态匹配，保证每个分区有独立worker
	partitionCount := c.opts.DesiredPartitions
	actualWorkerCount := workerCount
	if partitionCount > 0 && workerCount < partitionCount {
		actualWorkerCount = partitionCount
	}
	for i := 0; i < actualWorkerCount; i++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			for j := range jobs {
				// 记录开始时间
				operation := c.getOperationFromHeaders(j.msg.Headers)
				messageKey := string(j.msg.Key)
				processStart := time.Now()

				// 处理消息（带重试的业务处理）
				err := c.processMessageWithRetry(ctx, j.msg, 3)

				// 在本地记录指标（worker 负责记录处理耗时/成功/失败）
				c.recordConsumerMetrics(operation, messageKey, processStart, err, j.workerID)

				// 将处理结果返回给 fetcher，由 fetcher 负责提交偏移
				j.done <- err
			}
		}(i)
	}

	// fetcher: 负责从 Kafka 拉取消息，并在 worker 处理完成后提交偏移量
	fetchLoopDone := make(chan struct{})
	go func() {
		defer close(fetchLoopDone)
		nextWorker := 0
		// 批量聚合支持: 针对 create/update/delete，使用一个批量缓存由 worker 执行批量DB写
		type batchItem struct {
			op  string
			msg kafka.Message
		}

		// 单独的批处理队列 (用于批量DB写) — 由一个轻量 goroutine 管理定时刷新
		batchCh := make(chan batchItem, 4096) // 批量通道容量提升

		// 批量缓冲与提交 goroutine
		go func() {
			// 批量写入超时参数提升，默认50ms-200ms
			batchTimeout := c.opts.BatchTimeout
			if batchTimeout < 50*time.Millisecond {
				batchTimeout = 50 * time.Millisecond
			} else if batchTimeout > 200*time.Millisecond {
				batchTimeout = 200 * time.Millisecond
			}
			ticker := time.NewTicker(batchTimeout)
			defer ticker.Stop()
			var createBatch []kafka.Message
			var deleteBatch []kafka.Message
			var updateBatch []kafka.Message
			// 批量写入最大条数提升，默认100-500
			maxBatchSize := c.opts.MaxDBBatchSize
			if maxBatchSize < 100 {
				maxBatchSize = 100
			} else if maxBatchSize > 500 {
				maxBatchSize = 500
			}
			flush := func() {
				if len(createBatch) > 0 {
					c.batchCreateToDB(ctx, createBatch)
					createBatch = createBatch[:0]
				}
				if len(deleteBatch) > 0 {
					c.batchDeleteFromDB(ctx, deleteBatch)
					deleteBatch = deleteBatch[:0]
				}
				if len(updateBatch) > 0 {
					c.batchUpdateToDB(ctx, updateBatch)
					updateBatch = updateBatch[:0]
				}
			}
			for {
				select {
				case bi, ok := <-batchCh:
					if !ok {
						flush()
						return
					}
					switch bi.op {
					case OperationCreate:
						createBatch = append(createBatch, bi.msg)
						if len(createBatch) >= maxBatchSize {
							c.batchCreateToDB(ctx, createBatch)
							createBatch = createBatch[:0]
						}
					case OperationDelete:
						deleteBatch = append(deleteBatch, bi.msg)
						if len(deleteBatch) >= maxBatchSize {
							c.batchDeleteFromDB(ctx, deleteBatch)
							deleteBatch = deleteBatch[:0]
						}
					case OperationUpdate:
						updateBatch = append(updateBatch, bi.msg)
						if len(updateBatch) >= maxBatchSize {
							c.batchUpdateToDB(ctx, updateBatch)
							updateBatch = updateBatch[:0]
						}
					default:
						// ignore others for batching
					}
				case <-ticker.C:
					flush()
				case <-ctx.Done():
					flush()
					return
				}
			}
		}()

		// ====== 消费速率统计相关变量 ======
		var consumeCount int64 = 0

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// ====== 完全无限速，无日志 ======
			stats := c.reader.Stats()
			lag := stats.Lag
			if lag == 0 {
				select {
				case <-time.After(100 * time.Millisecond):
				case <-ctx.Done():
					return
				}
			}
			// ====== 结束 ======

			// FetchMessage 带重试：与之前逻辑保持一致
			var msg kafka.Message
			var fetchErr error
			for retry := 0; retry < c.opts.MaxRetries; retry++ {
				msg, fetchErr = c.reader.FetchMessage(ctx)
				if fetchErr == nil {
					break
				}
				if errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, context.DeadlineExceeded) {
					log.Debugf("Fetcher: 上下文已取消，停止获取消息")
					return
				}
				log.Warnf("Fetcher: 获取消息失败 (重试 %d/%d): %v", retry+1, c.opts.MaxRetries, fetchErr)
				backoff := time.Second * time.Duration(1<<uint(retry))
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					log.Debugf("Fetcher: 重试期间上下文取消")
					return
				}
			}
			if fetchErr != nil {
				log.Errorf("Fetcher: 获取消息最终失败: %v", fetchErr)
				// 在 fetch 失败时短暂停顿，避免紧循环
				select {
				case <-time.After(500 * time.Millisecond):
				case <-ctx.Done():
					return
				}
				continue
			}

			// dispatch to worker
			j := &job{msg: msg, done: make(chan error, 1), workerID: nextWorker}
			// ...existing code... // 移除队列长度监控，确保无限流
			select {
			case jobs <- j:
				// dispatched
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
				// 如果 worker 队列阻塞，尝试把消息放到批量通道以触发批量写（降低延迟）
				op := c.getOperationFromHeaders(msg.Headers)
				if op == OperationCreate || op == OperationDelete || op == OperationUpdate {
					select {
					case batchCh <- batchItem{op: op, msg: msg}:
					default:
						// 如果批量通道也满了，则继续等待正常 dispatch
						select {
						case jobs <- j:
						case <-ctx.Done():
							return
						}
					}
					continue
				}
			}

			// round-robin
			nextWorker = (nextWorker + 1) % workerCount

			// 等待 worker 完成处理
			procErr := <-j.done
			if procErr != nil {
				log.Warnf("Fetcher: message processing failed (worker=%d): %v", j.workerID, procErr)
				// 处理失败，不提交偏移量（与之前行为一致），继续下一个消息
				continue
			}

			// 处理成功后提交偏移
			if err := c.commitWithRetry(ctx, msg, j.workerID); err != nil {
				log.Errorf("Fetcher: 提交偏移失败: %v", err)
				// 提交失败则不阻塞 fetcher，继续下一条（commitWithRetry 内部已做重试）
			}

			// 消费速率统计：每处理一条消息计数+1
			consumeCount++
		}
	}()

	// 在 worker 与 fetcher 启动后标记就绪
	signalReady()

	// 等待 fetcher 结束（通常由 ctx 取消触发），然后关闭 jobs 并等待 workers 退出
	<-fetchLoopDone
	close(jobs)
	workerWg.Wait()
}

// 消息调度 - 已弃用
// StartConsuming 已经采用单 fetcher + worker 池的模式替代了旧的并发 Fetch/Commit 实现。
// 保留该函数签名以避免潜在外部引用编译错误，但实现为空。

// 处理消息
// ...old worker and processSingleMessage removed. Use StartConsuming with the new fetcher+worker flow.

// commitWithRetry 尝试提交消息偏移，遇到临时错误会重试
func (c *UserConsumer) commitWithRetry(ctx context.Context, msg kafka.Message, workerID int) error {
	maxAttempts := 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			lastErr = err
			metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, "commit", "commit_error").Inc()
			// record commit failure metric (partition as string)
			metrics.ConsumerCommitFailures.WithLabelValues(c.topic, c.groupID, fmt.Sprintf("%d", msg.Partition)).Inc()
			log.Warnf("Worker %d: 提交偏移量失败 (尝试 %d/%d): topic=%s partition=%d offset=%d err=%v",
				workerID, i+1, maxAttempts, msg.Topic, msg.Partition, msg.Offset, err)
			// 指数退避
			wait := time.Duration(100*(1<<uint(i))) * time.Millisecond
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			// record commit success metric
			metrics.ConsumerCommitSuccess.WithLabelValues(c.topic, c.groupID, fmt.Sprintf("%d", msg.Partition)).Inc()
			//	log.Debugf("Worker %d: 偏移量提交成功: topic=%s partition=%d offset=%d", workerID, msg.Topic, msg.Partition, msg.Offset)
			return nil
		}
	}
	log.Errorf("Worker %d: 提交偏移量最终失败: %v", workerID, lastErr)
	return lastErr
}

// 业务处理
func (c *UserConsumer) processMessage(ctx context.Context, msg kafka.Message) error {
	operation := c.getOperationFromHeaders(msg.Headers)

	switch operation {
	case OperationCreate:
		return c.processCreateOperation(ctx, msg)
	case OperationUpdate:
		return c.processUpdateOperation(ctx, msg)
	case OperationDelete:
		return c.processDeleteOperation(ctx, msg)
	default:
		log.Errorf("未知操作类型: %s", operation)
		if c.producer != nil {
			return c.producer.SendToDeadLetterTopic(ctx, msg, "UNKNOWN_OPERATION: "+operation)
		}
		return fmt.Errorf("未知操作类型: %s", operation)
	}
}

func (c *UserConsumer) processCreateOperation(ctx context.Context, msg kafka.Message) error {
	// 兼容两种结构：1. 扁平 v1.User 2. 带 metadata 的嵌套结构
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err == nil {
		if err := validation.ValidateUserFields(user.Name, user.Nickname, user.Password, user.Email, user.Phone); err != nil {
			// 字段校验失败，直接写入死信区
			return c.sendToDeadLetter(ctx, msg, err.Error())
		}
		log.Debugf("开始建立用户: username=%s", user.Name)
		if err := c.createUserInDB(ctx, &user); err != nil {
			// 数据库写入失败，直接写入死信区
			return c.sendToDeadLetter(ctx, msg, "CREATE_DB_ERROR: "+err.Error())
		}
		if err := c.setUserCache(ctx, &user, nil); err != nil {
			log.Warnf("用户创建成功但缓存设置失败: username=%s, error=%v", user.Name, err)
		} else {
			log.Debugf("用户%s缓存成功", user.Name)
		}
		log.Debugf("用户创建成功: username=%s", user.Name)
		return nil
	}

	return nil
}

// 删除
func (c *UserConsumer) processDeleteOperation(ctx context.Context, msg kafka.Message) error {

	var deleteRequest struct {
		Username  string `json:"username"`
		DeletedAt string `json:"deleted_at"`
	}

	if err := json.Unmarshal(msg.Value, &deleteRequest); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("开始删除用户: username=%s", deleteRequest.Username)

	var (
		userID           uint64
		existingSnapshot *v1.User
	)
	if deleteRequest.Username != "" {
		var existing v1.User
		if err := c.db.WithContext(ctx).
			Where("name = ?", deleteRequest.Username).
			First(&existing).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return c.sendToRetry(ctx, msg, "查询用户失败: "+err.Error())
			}
		} else {
			userID = existing.ID
			existingCopy := existing
			existingSnapshot = &existingCopy
		}
	}

	if err := c.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return c.sendToRetry(ctx, msg, "删除用户失败: "+err.Error())
	}

	c.purgeUserState(ctx, deleteRequest.Username, userID, existingSnapshot)
	log.Debugf("用户删除成功: username=%s", deleteRequest.Username)

	return nil
}

func (c *UserConsumer) processUpdateOperation(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}
	if err := validation.ValidateUserFields(user.Name, user.Nickname, user.Password, user.Email, user.Phone); err != nil {
		return c.sendToDeadLetter(ctx, msg, err.Error())
	}

	log.Debugf("处理用户更新: username=%s", user.Name)

	var existingSnapshot *v1.User
	var existing v1.User
	if err := c.db.WithContext(ctx).
		Where("name = ?", user.Name).
		First(&existing).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.sendToDeadLetter(ctx, msg, "UPDATE_TARGET_NOT_FOUND: "+user.Name)
		}
		return c.sendToRetry(ctx, msg, "查询用户失败: "+err.Error())
	}
	existingCopy := existing
	existingSnapshot = &existingCopy

	if err := c.updateUserInDB(ctx, &user); err != nil {
		return c.sendToRetry(ctx, msg, "更新用户失败: "+err.Error())
	}

	if err := c.setUserCache(ctx, &user, existingSnapshot); err != nil {
		log.Warnf("用户更新成功但缓存刷新失败: username=%s err=%v", user.Name, err)
	}

	log.Debugf("用户更新成功: username=%s", user.Name)
	return nil
}

func (c *UserConsumer) createUserInDB(ctx context.Context, user *v1.User) error {

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	//	log.Infof("[单条插入] 尝试插入用户: %s", user.Name)
	// 注意：这里直接使用 c.db，在集群模式下这是主库连接
	// 在单机模式下这是唯一数据库连接
	if err := c.db.WithContext(ctx).Create(user).Error; err != nil {
		//	metrics.DatabaseQueryErrors.WithLabelValues("create", "users", getErrorType(err)).Inc()
		return fmt.Errorf("数据创建失败: %v", err)
	}
	//	log.Infof("[单条插入] 成功: %s", user.Name)
	return nil
}

func (c *UserConsumer) deleteUserFromDB(ctx context.Context, username string) error {
	result := c.db.WithContext(ctx).
		Where("name = ? ", username).
		Delete(&v1.User{})
	if result.Error != nil {
		return result.Error
	}
	// 关键：检查实际影响行数
	if result.RowsAffected == 0 {
		return errors.WithCode(code.ErrUserNotFound, "用户没有发现")
	}
	return nil
}

func (c *UserConsumer) updateUserInDB(ctx context.Context, user *v1.User) error {
	user.UpdatedAt = time.Now()

	if err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", user.Name).
		Updates(map[string]interface{}{
			"email":     user.Email,
			"password":  user.Password,
			"status":    user.Status,
			"updatedAt": user.UpdatedAt,
		}).Error; err != nil {
		return fmt.Errorf("数据库更新失败: %v", err)
	}
	return nil
}

// 辅助函数
// processMessageWithRetry 带重试的消息处理
func (c *UserConsumer) processMessageWithRetry(ctx context.Context, msg kafka.Message, maxRetries int) error {
	log.Debugf("[Consumer] processMessageWithRetry: key=%s, maxRetries=%d", string(msg.Key), maxRetries)
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		//log.Debugf("开始第%d次处理消息", attempt)
		err := c.processMessage(ctx, msg)
		if err == nil {
			//	log.Debugf("第%d次处理成功", attempt)
			return nil // 处理成功,跳出循环
		}

		lastErr = err

		// 检查错误类型
		if !shouldRetry(err) {
			log.Warn("进入不可重试处理流程...")
			return nil //认为处理完成
		}

		// 可重试错误：记录日志并等待重试
		log.Warnf("消息处理失败，准备重试 (尝试 %d/%d): %v", attempt, maxRetries, err)

		if attempt < maxRetries {
			// 指数退避，但有上限
			backoff := c.calculateBackoff(attempt)
			log.Debugf("等待 %v 后进行第%d次重试", backoff, attempt+1)
			select {
			case <-time.After(backoff):
				// 继续重试
			case <-ctx.Done():
				return fmt.Errorf("重试期间上下文取消: %v", ctx.Err())
			}
		}
	}

	// 重试次数用尽，发送到重试主题
	log.Errorf("消息处理重试次数用尽: %v", lastErr)
	retryErr := c.sendToRetry(ctx, msg, fmt.Sprintf("重试次数用尽: %v", lastErr))
	if retryErr != nil {
		return fmt.Errorf("发送重试主题失败: %v (原错误: %v)", retryErr, lastErr)
	}

	log.Debugf("消息已发送到重试主题: %s", string(msg.Key))
	return nil // 重试主题发送成功，认为处理完成
}

// calculateBackoff 计算指数退避延迟时间
func (c *UserConsumer) calculateBackoff(attempt int) time.Duration {
	maxBackoff := 30 * time.Second
	minBackoff := 1 * time.Second

	// 指数退避公式：base * 2^(attempt-1)
	backoff := minBackoff * time.Duration(1<<uint(attempt-1))

	// 限制最大延迟
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}

// 记录消费信息
func (c *UserConsumer) recordConsumerMetrics(operation, messageKey string, processStart time.Time, processingErr error, workerID int) {
	processingDuration := time.Since(processStart).Seconds()

	// 添加详细的处理时间日志
	if processingErr != nil {
		log.Errorf("Worker %d 业务处理失败: topic=%s, key=%s, operation=%s, 处理耗时=%.3fs, 错误=%v",
			workerID, c.topic, messageKey, operation, processingDuration, processingErr)
	} else {
		//	log.Debugf("Worker %d 业务处理成功: topic=%s, operation=%s, 耗时=%.3fs",
		//	workerID, c.topic, operation, processingDuration)
	}

	// 记录消息接收（无论成功失败）
	if operation != "" {
		metrics.ConsumerMessagesReceived.WithLabelValues(c.topic, c.groupID, operation).Inc()
	}

	// 如果有错误，记录错误指标
	if processingErr != nil {
		if operation != "" {
			errorType := getErrorType(processingErr)
			metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, operation, errorType).Inc()
			metrics.ConsumerProcessingTime.WithLabelValues(c.topic, c.groupID, operation, "error").Observe(processingDuration)
		}
		return
	}

	// 记录成功处理
	if operation != "" {
		metrics.ConsumerMessagesProcessed.WithLabelValues(c.topic, c.groupID, operation).Inc()
		metrics.ConsumerProcessingTime.WithLabelValues(c.topic, c.groupID, operation, "success").Observe(processingDuration)
	}
}

// 添加错误类型提取函数
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "UNMARSHAL_ERROR"):
		return "unmarshal_error"
	case strings.Contains(errStr, "数据库"):
		return "database_error"
	case strings.Contains(errStr, "缓存"):
		return "cache_error"
	case strings.Contains(errStr, "context deadline exceeded"):
		return "timeout"
	default:
		return "unknown_error"
	}
}

func (c *UserConsumer) getOperationFromHeaders(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == HeaderOperation {
			return string(header.Value)
		}
	}
	return OperationCreate
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 第一层：明确不可重试的错误
	if isUnrecoverableError(errStr) {
		return false
	}

	// 第二层：明确可重试的错误
	if isRecoverableError(errStr) {
		return true
	}

	// 第三层：默认情况
	return false
}

// isUnrecoverableError 判断是否为不可恢复的错误
func isUnrecoverableError(errStr string) bool {
	unrecoverableErrors := []string{
		// 数据重复错误
		"Duplicate entry", "1062", "23000", "duplicate key value", "23505",
		"用户已存在", "UserAlreadyExist",

		// 消息格式错误
		"UNMARSHAL_ERROR", "invalid json", "unknown operation", "poison message",

		// 权限和DEFINER错误
		"definer", "DEFINER", "1449", "permission denied",

		// 数据不存在错误（幂等性）
		"does not exist", "not found", "record not found", "ErrRecordNotFound",

		// 数据库约束错误
		"constraint", "foreign key", "1451", "1452", "syntax error",

		// 字段超长错误
		"Data too long for column", "1406",

		// GORM 相关不可重试错误
		"ErrInvalidData", "ErrInvalidTransaction", "ErrNotImplemented", "ErrMissingWhereClause", "ErrPrimaryKeyRequired", "ErrModelValueRequired", "ErrUnsupportedRelation", "ErrRegistered", "ErrInvalidField", "ErrEmptySlice", "ErrDryRunModeUnsupported",

		// 业务逻辑错误
		"invalid format", "validation failed",
	}

	for _, unrecoverableErr := range unrecoverableErrors {
		if strings.Contains(errStr, unrecoverableErr) {
			return true
		}
	}
	return false
}

// isRecoverableError 判断是否为可恢复的错误
func isRecoverableError(errStr string) bool {
	recoverableErrors := []string{
		// 超时和网络错误
		"timeout", "deadline exceeded", "connection refused", "network error",
		"connection reset", "broken pipe", "no route to host",

		// 数据库临时错误
		"database is closed", "deadlock", "1213", "40001",
		"temporary", "busy", "lock", "try again",

		// 资源暂时不可用
		"resource temporarily unavailable", "too many connections",

		// GORM 可重试错误
		"ErrInvalidTransaction", "ErrDryRunModeUnsupported",
	}

	for _, recoverableErr := range recoverableErrors {
		if strings.Contains(errStr, recoverableErr) {
			return true
		}
	}
	return false
}

func (c *UserConsumer) setUserCache(ctx context.Context, user *v1.User, previous *v1.User) error {
	startTime := time.Now()
	var operationErr error
	defer func() {
		metrics.RecordRedisOperation("set", time.Since(startTime).Seconds(), operationErr)
	}()

	cacheKey := usercache.UserKey(user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		operationErr = err
		return err
	}
	operationErr = c.redis.SetKey(ctx, cacheKey, string(data), 24*time.Hour)
	if previous != nil {
		c.evictContactCaches(ctx, previous, user)
	}
	c.writeContactCaches(ctx, user)
	return operationErr
}

func (c *UserConsumer) deleteUserCache(ctx context.Context, username string) error {
	cacheKey := usercache.UserKey(username)
	if cacheKey == "" {
		return nil
	}
	if _, err := c.redis.DeleteKey(ctx, cacheKey); err != nil {
		return err
	}
	log.Debugf("删除用户%s cacheKey:%s成功", username, cacheKey)
	return nil
}

func (c *UserConsumer) purgeUserState(ctx context.Context, username string, userID uint64, snapshot *v1.User) {
	if err := c.deleteUserCache(ctx, username); err != nil {
		log.Errorw("缓存删除失败", "username", username, "error", err)
	} else {
		log.Debugf("缓存删除成功: username=%s", username)
	}

	if snapshot != nil {
		c.evictContactCaches(ctx, snapshot, nil)
	}

	if userID == 0 {
		return
	}

	if err := cleanupUserSessions(ctx, c.redis, userID); err != nil {
		log.Errorw("刷新令牌清理失败", "username", username, "userID", userID, "error", err)
		return
	}
	log.Debugf("刷新令牌清理成功: username=%s userID=%d", username, userID)
}

func (c *UserConsumer) evictContactCaches(ctx context.Context, previous *v1.User, current *v1.User) {
	if previous == nil {
		return
	}
	prevEmail := usercache.NormalizeEmail(previous.Email)
	curEmail := ""
	if current != nil {
		curEmail = usercache.NormalizeEmail(current.Email)
	}
	if prevEmail != "" && prevEmail != curEmail {
		c.removeCacheKey(ctx, usercache.EmailKey(previous.Email))
	}

	prevPhone := usercache.NormalizePhone(previous.Phone)
	curPhone := ""
	if current != nil {
		curPhone = usercache.NormalizePhone(current.Phone)
	}
	if prevPhone != "" && prevPhone != curPhone {
		c.removeCacheKey(ctx, usercache.PhoneKey(previous.Phone))
	}
}

func (c *UserConsumer) writeContactCaches(ctx context.Context, user *v1.User) {
	if user == nil {
		return
	}
	if key := usercache.EmailKey(user.Email); key != "" {
		if err := c.redis.SetKey(ctx, key, user.Name, 24*time.Hour); err != nil {
			log.Warnf("邮箱缓存写入失败: username=%s key=%s err=%v", user.Name, key, err)
		}
	}
	if key := usercache.PhoneKey(user.Phone); key != "" {
		if err := c.redis.SetKey(ctx, key, user.Name, 24*time.Hour); err != nil {
			log.Warnf("手机号缓存写入失败: username=%s key=%s err=%v", user.Name, key, err)
		}
	}
}

func (c *UserConsumer) removeCacheKey(ctx context.Context, cacheKey string) {
	if cacheKey == "" {
		return
	}
	if _, err := c.redis.DeleteKey(ctx, cacheKey); err != nil {
		log.Warnf("缓存删除失败: key=%s err=%v", cacheKey, err)
	}
}

// 发送到重试主题
func (c *UserConsumer) sendToRetry(ctx context.Context, msg kafka.Message, errorInfo string) error {

	operation := c.getOperationFromHeaders(msg.Headers)

	errorType := getErrorType(fmt.Errorf("%s", errorInfo))
	// 记录重试指标
	metrics.ConsumerRetryMessages.WithLabelValues(c.topic, c.groupID, operation, errorType).Inc()

	log.Debugf("🔄 准备发送到重试主题: key=%s, error=%s", string(msg.Key), errorInfo)
	log.Debugf("  原始消息Headers: %+v", msg.Headers)
	if c.producer == nil {
		return fmt.Errorf("producer未初始化")
	}

	// ✅ 确保这里传递原始消息的Headers
	retryMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: msg.Headers, // 直接使用原始Headers
		Time:    time.Now(),
	}

	retryMsg.Headers = append(retryMsg.Headers, kafka.Header{
		Key:   HeaderRetryError,
		Value: []byte(errorInfo),
	})

	return c.producer.sendToRetryTopic(ctx, retryMsg, errorInfo)
}

func (c *UserConsumer) sendToDeadLetter(ctx context.Context, msg kafka.Message, reason string) error {
	operation := c.getOperationFromHeaders(msg.Headers)
	errorType := getErrorType(fmt.Errorf("%s", reason))
	// 记录死信指标
	metrics.ConsumerDeadLetterMessages.WithLabelValues(c.topic, c.groupID, operation, errorType).Inc()
	if c.producer == nil {
		return fmt.Errorf("producer未初始化")
	}
	return c.producer.SendToDeadLetterTopic(ctx, msg, reason)
}

// 修改 startLagMonitor 方法
// func (c *UserConsumer) startLagMonitor(ctx context.Context) {
// 	go func() {
// 		ticker := time.NewTicker(c.opts.LagCheckInterval)
// 		defer ticker.Stop()

// 		for {
// 			select {
// 			case <-ticker.C:
// 				// 直接获取统计信息，不需要检查 nil
// 				stats := c.reader.Stats()
// 				metrics.ConsumerLag.WithLabelValues(c.topic, c.groupID).Set(float64(stats.Lag))
// 				// 1. 每个实例定期上报自己的 lag 到 Redis
// 				instanceKey := fmt.Sprintf("kafka:lag:%s:%s:%d", c.topic, c.groupID, c.instanceID)
// 				err := c.redis.SetKey(ctx, instanceKey, fmt.Sprintf("%d", stats.Lag), 2*c.opts.LagCheckInterval)
// 				if err != nil {
// 					log.Errorf("[LagMonitor] 写入Redis失败: key=%s, lag=%d, err=%v", instanceKey, stats.Lag, err)
// 				} else {
// 					//		log.Debugf("[LagMonitor] 写入Redis: key=%s, lag=%d", instanceKey, stats.Lag)
// 				}

// 				// 主控选举：用 Redis 分布式锁，锁定 2*LagCheckInterval
// 				masterKey := fmt.Sprintf("kafka:lag:master:%s:%s", c.topic, c.groupID)
// 				lockVal := fmt.Sprintf("%d", c.instanceID)
// 				// 尝试抢占主控
// 				gotLock := false
// 				if !c.isMaster {
// 					success, err := c.redis.SetNX(ctx, masterKey, lockVal, 2*c.opts.LagCheckInterval)
// 					if err == nil && success {
// 						c.isMaster = true
// 						gotLock = true
// 						//				log.Debugf("[LagMonitor] 成为主控: masterKey=%s, val=%s", masterKey, lockVal)
// 					}
// 				} else {
// 					// 检查自己是否还是主控
// 					v, err := c.redis.GetKey(ctx, masterKey)
// 					if err == nil && v == lockVal {
// 						gotLock = true
// 					} else {
// 						c.isMaster = false
// 						//						log.Debugf("[LagMonitor] 主控失效: masterKey=%s, val=%s, err=%v", masterKey, v, err)

// 					}
// 				}

// 				// 全局聚合所有相关组 lag
// 				if gotLock {
// 					groups := []string{"user-service-prod.create", "user-service-prod.update", "user-service-prod.delete"}
// 					totalLag := int64(0)
// 					for _, group := range groups {
// 						keys := c.redis.GetKeys(ctx, fmt.Sprintf("kafka:lag:%s:%s:*", c.topic, group))
// 						for _, k := range keys {
// 							v, err := c.redis.GetKey(ctx, k)
// 							if err == nil {
// 								var lag int64
// 								fmt.Sscanf(v, "%d", &lag)
// 								totalLag += lag
// 							}
// 						}
// 					}
// 					protectTTL := 2 * c.opts.LagCheckInterval
// 					globalProtectKey := "kafka:lag:protect:ALL"
// 					if totalLag >= c.opts.LagScaleThreshold {
// 						_ = c.redis.SetKey(ctx, globalProtectKey, "1", protectTTL)
// 					} else {
// 						_ = c.redis.SetKey(ctx, globalProtectKey, "0", protectTTL)
// 					}
// 					//	log.Warnf("[全局保护] totalLag=%d, threshold=%d, master=%v", totalLag, c.opts.LagScaleThreshold, c.instanceID)
// 				}

// 				// 3. 所有实例消费前检查保护信号
// 				v, err := c.redis.GetKey(ctx, "kafka:lag:protect:ALL")
// 				if err == nil && v == "1" {
// 					metrics.ConsumerLag.WithLabelValues(c.topic, c.groupID).Set(1)
// 					// 这里可直接 return 或 sleep，阻断消费
// 				} else {
// 					metrics.ConsumerLag.WithLabelValues(c.topic, c.groupID).Set(0)
// 				}
// 			case <-ctx.Done():
// 				return
// 			}
// 		}
// 	}()
// }

// batchCreateToDB 使用 GORM 批量创建用户实体
func (c *UserConsumer) batchCreateToDB(ctx context.Context, msgs []kafka.Message) {
	log.Debugf("[Consumer] batchCreateToDB: msgs=%d", len(msgs))
	if len(msgs) == 0 {
		return
	}
	start := time.Now()
	metrics.BusinessOperationsTotal.WithLabelValues("consumer", "batch_create", "kafka").Inc()
	metrics.BusinessInProgress.WithLabelValues("consumer", "batch_create").Inc()
	defer metrics.BusinessInProgress.WithLabelValues("consumer", "batch_create").Dec()
	var users []v1.User
	for _, m := range msgs {
		var u v1.User
		if err := json.Unmarshal(m.Value, &u); err != nil {
			log.Errorf("批量创建: 反序列化失败: %v", err)
			if c.producer != nil {
				_ = c.producer.SendToDeadLetterTopic(ctx, m, "BATCH_UNMARSHAL_ERROR: "+err.Error())
			}
			continue
		}
		if err := validation.ValidateUserFields(u.Name, u.Nickname, u.Password, u.Email, u.Phone); err != nil {
			log.Errorf("批量创建: %v", err)
			if c.producer != nil {
				_ = c.producer.SendToDeadLetterTopic(ctx, m, err.Error())
			}
			continue
		}
		now := time.Now()
		u.CreatedAt = now
		u.UpdatedAt = now
		users = append(users, u)
	}

	usernames := make([]string, 0, len(msgs))
	for _, m := range msgs {
		var u v1.User
		if err := json.Unmarshal(m.Value, &u); err == nil {
			usernames = append(usernames, u.Name)
		}
	}
	log.Infof("[批量插入] 尝试插入用户: %v", usernames)

	if len(users) == 0 {
		return
	}
	var opErr error
	if err := c.db.WithContext(ctx).Create(&users).Error; err != nil {
		opErr = err
		log.Errorf("[批量插入] 失败: %v, 用户: %v", err, usernames)
		metrics.BusinessFailures.WithLabelValues("consumer", "batch_create", getErrorType(err)).Inc()
		for _, m := range msgs {
			if c.producer != nil {
				_ = c.producer.sendToRetryTopic(ctx, m, "BATCH_CREATE_DB_ERROR: "+err.Error())
			}
		}
	} else {
		metrics.BusinessSuccess.WithLabelValues("consumer", "batch_create", "success").Inc()
		log.Infof("[批量插入] 成功: %v", usernames)
		log.Debugf("批量创建成功: %d 条记录", len(users))
		// 批量写入缓存
		for i := range users {
			if err := c.setUserCache(ctx, &users[i], nil); err != nil {
				log.Warnf("批量创建后缓存设置失败: username=%s, error=%v", users[i].Name, err)
			} else {
				log.Debugf("批量创建后缓存成功: username=%s", users[i].Name)
			}
		}
	}
	duration := time.Since(start).Seconds()
	metrics.BusinessProcessingTime.WithLabelValues("consumer", "batch_create").Observe(duration)
	metrics.BusinessThroughputStats.WithLabelValues("consumer", "batch_create").Observe(duration)
	if opErr != nil {
		errorRate := 1.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_create").Set(errorRate)
	} else {
		errorRate := 0.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_create").Set(errorRate)
	}
}

// batchDeleteFromDB 批量删除用户（按 username）
func (c *UserConsumer) batchDeleteFromDB(ctx context.Context, msgs []kafka.Message) {
	log.Debugf("[Consumer] batchDeleteFromDB: msgs=%d", len(msgs))
	if len(msgs) == 0 {
		return
	}
	start := time.Now()
	metrics.BusinessOperationsTotal.WithLabelValues("consumer", "batch_delete", "kafka").Inc()
	metrics.BusinessInProgress.WithLabelValues("consumer", "batch_delete").Inc()
	defer metrics.BusinessInProgress.WithLabelValues("consumer", "batch_delete").Dec()
	var usernames []string
	cleanupTargets := make(map[string]uint64)
	snapshots := make(map[string]*v1.User)
	for _, m := range msgs {
		var deleteRequest struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}
		if err := json.Unmarshal(m.Value, &deleteRequest); err != nil {
			log.Errorf("批量删除: 反序列化失败: %v", err)
			if c.producer != nil {
				_ = c.producer.SendToDeadLetterTopic(ctx, m, "BATCH_UNMARSHAL_ERROR: "+err.Error())
			}
			continue
		}
		usernames = append(usernames, deleteRequest.Username)
	}
	if len(usernames) == 0 {
		return
	}
	type userIdentifier struct {
		ID    uint64
		Name  string `gorm:"column:name"`
		Email string `gorm:"column:email"`
		Phone string `gorm:"column:phone"`
	}
	var identifiers []userIdentifier
	if err := c.db.WithContext(ctx).
		Model(&v1.User{}).
		Select("id", "name", "email", "phone").
		Where("name IN ?", usernames).
		Find(&identifiers).Error; err != nil {
		log.Warnf("批量删除前查询用户ID失败: %v", err)
	} else {
		for _, item := range identifiers {
			cleanupTargets[item.Name] = item.ID
			snapshots[item.Name] = &v1.User{
				Email: item.Email,
				Phone: item.Phone,
			}
		}
	}
	var opErr error
	if err := c.db.WithContext(ctx).Where("name IN ?", usernames).Delete(&v1.User{}).Error; err != nil {
		opErr = err
		log.Errorf("批量删除用户失败: %v", err)
		metrics.BusinessFailures.WithLabelValues("consumer", "batch_delete", getErrorType(err)).Inc()
		for _, m := range msgs {
			if c.producer != nil {
				_ = c.producer.sendToRetryTopic(ctx, m, "BATCH_DELETE_DB_ERROR: "+err.Error())
			}
		}
	} else {
		metrics.BusinessSuccess.WithLabelValues("consumer", "batch_delete", "success").Inc()
		log.Debugf("批量删除成功: %d 条记录", len(usernames))
		// 批量删除缓存
		for _, username := range usernames {
			c.purgeUserState(ctx, username, cleanupTargets[username], snapshots[username])
		}
	}
	duration := time.Since(start).Seconds()
	metrics.BusinessProcessingTime.WithLabelValues("consumer", "batch_delete").Observe(duration)
	metrics.BusinessThroughputStats.WithLabelValues("consumer", "batch_delete").Observe(duration)
	if opErr != nil {
		errorRate := 1.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_delete").Set(errorRate)
	} else {
		errorRate := 0.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_delete").Set(errorRate)
	}
}

// 唯一新增的方法
func (c *UserConsumer) SetInstanceID(id int) {
	c.instanceID = id
}

func (c *UserConsumer) SetProducer(producer *UserProducer) {
	c.producer = producer
}

func (c *UserConsumer) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

// batchUpdateToDB 批量更新用户（按 username）
func (c *UserConsumer) batchUpdateToDB(ctx context.Context, msgs []kafka.Message) {
	log.Debugf("[Consumer] batchUpdateToDB: msgs=%d", len(msgs))
	if len(msgs) == 0 {
		return
	}
	start := time.Now()
	metrics.BusinessOperationsTotal.WithLabelValues("consumer", "batch_update", "kafka").Inc()
	metrics.BusinessInProgress.WithLabelValues("consumer", "batch_update").Inc()
	defer metrics.BusinessInProgress.WithLabelValues("consumer", "batch_update").Dec()
	var opErr error
	var updatedCount int
	for _, m := range msgs {
		var u v1.User
		if err := json.Unmarshal(m.Value, &u); err != nil {
			log.Errorf("批量更新: 反序列化失败: %v", err)
			if c.producer != nil {
				_ = c.producer.SendToDeadLetterTopic(ctx, m, "BATCH_UNMARSHAL_ERROR: "+err.Error())
			}
			continue
		}
		if err := validation.ValidateUserFields(u.Name, u.Nickname, u.Password, u.Email, u.Phone); err != nil {
			log.Errorf("批量更新: %v", err)
			if c.producer != nil {
				_ = c.producer.SendToDeadLetterTopic(ctx, m, err.Error())
			}
			continue
		}
		u.UpdatedAt = time.Now()
		var existing v1.User
		if err := c.db.WithContext(ctx).
			Where("name = ?", u.Name).
			First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Warnf("批量更新目标不存在: %s", u.Name)
				if c.producer != nil {
					_ = c.producer.SendToDeadLetterTopic(ctx, m, "BATCH_UPDATE_TARGET_NOT_FOUND: "+u.Name)
				}
				continue
			}
			opErr = err
			log.Errorf("批量更新前查询失败: %v, 用户: %s", err, u.Name)
			metrics.BusinessFailures.WithLabelValues("consumer", "batch_update", getErrorType(err)).Inc()
			if c.producer != nil {
				_ = c.producer.sendToRetryTopic(ctx, m, "BATCH_UPDATE_QUERY_ERROR: "+err.Error())
			}
			continue
		}
		existingCopy := existing
		if err := c.db.WithContext(ctx).Model(&v1.User{}).
			Where("name = ?", u.Name).
			Updates(map[string]interface{}{
				"email":     u.Email,
				"password":  u.Password,
				"status":    u.Status,
				"updatedAt": u.UpdatedAt,
			}).Error; err != nil {
			opErr = err
			log.Errorf("批量更新失败: %v, 用户: %s", err, u.Name)
			metrics.BusinessFailures.WithLabelValues("consumer", "batch_update", getErrorType(err)).Inc()
			if c.producer != nil {
				_ = c.producer.sendToRetryTopic(ctx, m, "BATCH_UPDATE_DB_ERROR: "+err.Error())
			}
			continue
		}
		updatedCount++
		metrics.BusinessSuccess.WithLabelValues("consumer", "batch_update", "success").Inc()
		_ = c.setUserCache(ctx, &u, &existingCopy)
	}
	duration := time.Since(start).Seconds()
	metrics.BusinessProcessingTime.WithLabelValues("consumer", "batch_update").Observe(duration)
	metrics.BusinessThroughputStats.WithLabelValues("consumer", "batch_update").Observe(duration)
	if opErr != nil {
		errorRate := 1.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_update").Set(errorRate)
	} else {
		errorRate := 0.0
		metrics.BusinessErrorRate.WithLabelValues("consumer", "batch_update").Set(errorRate)
	}
	log.Infof("[批量更新] 成功: %d 条记录", updatedCount)
}
