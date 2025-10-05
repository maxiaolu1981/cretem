// internal/pkg/server/consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
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
}

func NewUserConsumer(brokers []string, topic, groupID string, db *gorm.DB, redis *storage.RedisCluster) *UserConsumer {
	consumer := &UserConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,

			// 优化配置
			MinBytes:      1 * 1024 * 1024,        // 降低最小字节数，立即消费
			MaxBytes:      10e6,                   // 10MB
			MaxWait:       time.Millisecond * 100, // 增加到100ms
			QueueCapacity: 100,                    // 降低队列容量，避免消息堆积在内存

			CommitInterval: 0,
			StartOffset:    kafka.FirstOffset,

			// 添加重试配置
			MaxAttempts:    3,
			ReadBackoffMin: time.Millisecond * 100,
			ReadBackoffMax: time.Millisecond * 1000,
		}),
		db:      db,
		redis:   redis,
		topic:   topic,
		groupID: groupID,
	}
	go consumer.startLagMonitor(context.Background())
	return consumer
}

// 消费
func (c *UserConsumer) StartConsuming(ctx context.Context, workerCount int) {
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.worker(ctx, workerID)
		}(i)
	}
	wg.Wait()
}

// 消息调度
func (c *UserConsumer) worker(ctx context.Context, workerID int) {
	log.Debugf("启动消费者实例 %d, Worker %d, Topic: %s, 消费组: %s",
		c.instanceID, workerID, c.topic, c.groupID)

	for {
		select {
		case <-ctx.Done():
			log.Debugf("Worker %d: 停止消费", workerID)
			return

		default:
			log.Debugf("Worker %d: 开始处理消息", workerID)
			err := c.processSingleMessage(ctx, workerID)

			if err != nil {
				log.Warnf("Worker %d: 处理消息失败: %v", workerID, err)
				// 所有错误都只是休眠后继续，不停止worker。使用指数退避以避免紧循环。
				time.Sleep(200 * time.Millisecond)
			} else {
				log.Debugf("Worker %d: 消息处理完成", workerID)
			}
		}
	}
}

// 处理消息
func (c *UserConsumer) processSingleMessage(ctx context.Context, workerID int) error {

	var operation, messageKey string
	var processingErr error
	var msg kafka.Message
	var processStart time.Time //用于记录真正的处理开始时间

	// 从Kafka拉取消息，添加重试逻辑
	var fetchErr error
	for retry := 0; retry < 3; retry++ {
		msg, fetchErr = c.reader.FetchMessage(ctx)
		if fetchErr == nil {
			break
		}

		// 检查是否是上下文取消
		if errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, context.DeadlineExceeded) {
			log.Debugf("Worker %d: 上下文已取消，停止获取消息", workerID)
			processingErr = fetchErr
			return fetchErr
		}
		log.Warnf("Worker %d: 获取消息失败 (重试 %d/3): %v", workerID, retry+1, fetchErr)
		// 指数退避
		backoff := time.Second * time.Duration(1<<uint(retry))
		select {
		case <-time.After(backoff):
			// 继续重试
		case <-ctx.Done():
			log.Debugf("Worker %d: 重试期间上下文取消", workerID)
			processingErr = ctx.Err()
			return processingErr
		}
	}

	if fetchErr != nil {
		log.Errorf("Worker %d: 获取消息最终失败: %v", workerID, fetchErr)
		processingErr = fetchErr
		return fetchErr
	}

	operation = c.getOperationFromHeaders(msg.Headers)
	messageKey = string(msg.Key)

	// 🔥 从这里开始计时真正的处理时间（不包括等待消息的时间
	processStart = time.Now()
	defer func() {
		c.recordConsumerMetrics(operation, messageKey, processStart, processingErr, workerID)
	}()

	// 处理消息（包含业务逻辑重试）
	processingErr = c.processMessageWithRetry(ctx, msg, 3)
	if processingErr != nil {
		log.Errorf("Worker %d: 消息处理最终失败: %v", workerID, processingErr)
		return processingErr
	}

	// 提交偏移量（确认消费）
	if err := c.commitWithRetry(ctx, msg, workerID); err != nil {
		return err
	}
	return nil
}

// commitWithRetry 尝试提交消息偏移，遇到临时错误会重试
func (c *UserConsumer) commitWithRetry(ctx context.Context, msg kafka.Message, workerID int) error {
	maxAttempts := 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			lastErr = err
			metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, "commit", "commit_error").Inc()
			log.Warnf("Worker %d: 提交偏移量失败 (尝试 %d/%d): %v", workerID, i+1, maxAttempts, err)
			// 指数退避
			wait := time.Duration(100*(1<<uint(i))) * time.Millisecond
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			log.Debugf("Worker %d: 偏移量提交成功", workerID)
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

	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return fmt.Errorf("UNMARSHAL_ERROR: %w", err) // 返回错误，让上层决定重试或死信
	}

	log.Debugf("开始建立用户: username=%s", user.Name)

	// 创建用户
	if err := c.createUserInDB(ctx, &user); err != nil {
		return fmt.Errorf("创建用户失败: %w", err) // 返回错误，让上层根据错误类型决定
	}

	// 设置缓存
	if err := c.setUserCache(ctx, &user); err != nil {
		log.Warnf("用户创建成功但缓存设置失败: username=%s, error=%v", user.Name, err)
		// 缓存失败不影响主流程，不返回错误
	} else {
		log.Debugf("用户%s缓存成功", user.Name)
	}

	log.Debugf("用户创建成功: username=%s", user.Name)
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

	if err := c.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return c.sendToRetry(ctx, msg, "删除用户失败: "+err.Error())
	}

	if err := c.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("缓存删除失败", "username", deleteRequest.Username, "error", err)
	} else {
		log.Debugf("缓存删除成功: username=%s", deleteRequest.Username)
	}
	log.Debugf("用户删除成功: username=%s", deleteRequest.Username)

	return nil
}

func (c *UserConsumer) processUpdateOperation(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("处理用户更新: username=%s", user.Name)

	if err := c.updateUserInDB(ctx, &user); err != nil {
		return c.sendToRetry(ctx, msg, "更新用户失败: "+err.Error())
	}

	c.setUserCache(ctx, &user)

	log.Debugf("用户更新成功: username=%s", user.Name)
	return nil
}

func (c *UserConsumer) createUserInDB(ctx context.Context, user *v1.User) error {

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// 注意：这里直接使用 c.db，在集群模式下这是主库连接
	// 在单机模式下这是唯一数据库连接
	if err := c.db.WithContext(ctx).Create(user).Error; err != nil {
		//	metrics.DatabaseQueryErrors.WithLabelValues("create", "users", getErrorType(err)).Inc()
		return fmt.Errorf("数据创建失败: %v", err)
	}
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
			"email":      user.Email,
			"password":   user.Password,
			"status":     user.Status,
			"updated_at": user.UpdatedAt,
		}).Error; err != nil {
		return fmt.Errorf("数据库更新失败: %v", err)
	}
	return nil
}

// 辅助函数
// processMessageWithRetry 带重试的消息处理
func (c *UserConsumer) processMessageWithRetry(ctx context.Context, msg kafka.Message, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Debugf("开始第%d次处理消息", attempt)
		err := c.processMessage(ctx, msg)
		if err == nil {
			log.Debugf("第%d次处理成功", attempt)
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
		log.Debugf("Worker %d 业务处理成功: topic=%s, operation=%s, 耗时=%.3fs",
			workerID, c.topic, operation, processingDuration)
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
		"does not exist", "not found", "record not found",

		// 数据库约束错误
		"constraint", "foreign key", "1451", "1452", "syntax error",

		// 业务逻辑错误
		"invalid format", "validation failed",
	}

	for _, unrecoverableErr := range unrecoverableErrors {
		if strings.Contains(errStr, unrecoverableErr) {
			return false
		}
	}
	return true
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
	}

	for _, recoverableErr := range recoverableErrors {
		if strings.Contains(errStr, recoverableErr) {
			return true
		}
	}
	return false
}

func (c *UserConsumer) setUserCache(ctx context.Context, user *v1.User) error {
	startTime := time.Now()
	var operationErr error // 用于记录最终的操作错误
	defer func() {
		// 使用defer确保无论从哪个return退出都会记录指标
		metrics.RecordRedisOperation("set", time.Since(startTime).Seconds(), operationErr)
	}()

	cacheKey := fmt.Sprintf("user:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		operationErr = err
		return err
	}
	operationErr = c.redis.SetKey(ctx, cacheKey, string(data), 24*time.Hour)
	return operationErr
}
func (c *UserConsumer) deleteUserCache(ctx context.Context, username string) error {
	cacheKey := fmt.Sprintf("user:%s", username)
	_, err := c.redis.DeleteKey(ctx, cacheKey)
	if err != nil {
		return err
	}
	log.Debugf("删除:%s成功", cacheKey)
	return nil
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
func (c *UserConsumer) startLagMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 直接获取统计信息，不需要检查 nil
				stats := c.reader.Stats()
				metrics.ConsumerLag.WithLabelValues(c.topic, c.groupID).Set(float64(stats.Lag))
				// 可选：记录调试日志
				if stats.Lag > 0 {
					log.Debugf("消费者延迟: topic=%s, group=%s, lag=%d", c.topic, c.groupID, stats.Lag)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
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
