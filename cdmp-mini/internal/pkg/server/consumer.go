// internal/pkg/server/consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
			StartOffset:    kafka.LastOffset,

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

func (c *UserConsumer) SetProducer(producer *UserProducer) {
	c.producer = producer
}

func (c *UserConsumer) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

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

func (c *UserConsumer) worker(ctx context.Context, workerID int) {
	log.Infof("启动消费者实例 %d, Worker %d, Topic: %s, 消费组: %s",
		c.instanceID, workerID, c.topic, c.groupID)

	// 添加健康检查计数器
	healthCheckTicker := time.NewTicker(30 * time.Second)
	defer healthCheckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Infof("Worker %d: 停止消费", workerID)
			return
		case <-healthCheckTicker.C:
			stats := c.reader.Stats()
			log.Infof("Worker %d: 消费者状态 - Lag: %d, 错误数: %d", workerID, stats.Lag, stats.Errors)

			// 添加告警
			if stats.Lag > 1000 {
				log.Errorf("Worker %d: 消费延迟过高! Lag: %d", workerID, stats.Lag)
			}
			if stats.Errors > 10 {
				log.Errorf("Worker %d: 错误数过多! Errors: %d", workerID, stats.Errors)
			}
		default:
			log.Debugf("Worker %d: 开始处理消息", workerID)
			err := c.processSingleMessage(ctx, workerID)

			if err != nil {
				log.Warnf("Worker %d: 处理消息失败: %v", workerID, err)
				// 所有错误都只是休眠后继续，不停止worker
				time.Sleep(100 * time.Millisecond)
			} else {
				log.Debugf("Worker %d: 消息处理完成", workerID)
			}
		}
	}
}

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
			log.Infof("Worker %d: 上下文已取消，停止获取消息", workerID)
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
			log.Infof("Worker %d: 重试期间上下文取消", workerID)
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
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		log.Errorf("Worker %d: 提交偏移量失败: %v", workerID, err)
		metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, "commit", "commit_error").Inc()
		return err
	} else {
		log.Debugf("Worker %d: 偏移量提交成功", workerID)
	}
	return nil

}

// 判断是否需要停止worker的严重错误
func (c *UserConsumer) shouldStopWorker(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// 这些错误需要停止worker
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "broker not available") ||
		strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "authorization failed")
}

// processMessageWithRetry 带重试的消息处理
func (c *UserConsumer) processMessageWithRetry(ctx context.Context, msg kafka.Message, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Info("开始根据消息处理业务......")
		err := c.processMessage(ctx, msg)
		if err == nil {
			return nil // 处理成功
		}

		lastErr = err
		errorType := getErrorType(err)

		// 检查是否应该重试
		if !shouldRetry(err) {
			log.Warnf("消息处理遇到不可重试错误 (尝试 %d/%d): %v", attempt, maxRetries, err)

			// 不可重试错误：发送到死信主题
			deadLetterErr := c.sendToDeadLetter(ctx, msg, fmt.Sprintf("不可重试错误[%s]: %v", errorType, err))
			if deadLetterErr != nil {
				return fmt.Errorf("发送死信失败: %v (原错误: %v)", deadLetterErr, err)
			}

			log.Infof("消息已发送到死信主题: %s", string(msg.Key))
			return nil // 死信发送成功，认为处理完成
		}

		// 可重试错误：记录日志并等待重试
		log.Warnf("消息处理失败，准备重试 (尝试 %d/%d): %v", attempt, maxRetries, err)

		if attempt < maxRetries {
			// 指数退避
			backoff := time.Second * time.Duration(1<<uint(attempt-1))
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

	log.Infof("消息已发送到重试主题: %s", string(msg.Key))
	return nil // 重试主题发送成功，认为处理完成
}

func (c *UserConsumer) recordConsumerMetrics(operation, messageKey string, processStart time.Time, processingErr error, workerID int) {
	processingDuration := time.Since(processStart).Seconds()

	// 添加详细的处理时间日志
	if processingErr != nil {
		log.Debugf("Worker %d 业务处理失败: topic=%s, key=%s, operation=%s, 处理耗时=%.3fs, 错误=%v",
			workerID, c.topic, messageKey, operation, processingDuration, processingErr)
	} else {
		log.Infof("Worker %d 业务处理成功: topic=%s, operation=%s, 耗时=%.3fs",
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

func (c *UserConsumer) getOperationFromHeaders(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == HeaderOperation {
			return string(header.Value)
		}
	}
	return OperationCreate
}

func (c *UserConsumer) processCreateOperation(ctx context.Context, msg kafka.Message) error {

	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return fmt.Errorf("UNMARSHAL_ERROR: %w", err) // 返回错误，让上层决定重试或死信
	}

	log.Debugf("开始建立用户: username=%s", user.Name)

	// 前置检查：用户是否已存在（避免不必要的数据库插入）
	exists, err := c.checkUserExists(ctx, user.Name)
	if err != nil {
		return fmt.Errorf("检查用户存在性失败: %w", err) // 返回错误，可重试
	}
	if exists {
		log.Warnf("用户已存在，跳过创建: username=%s", user.Name)
		return nil
	}
	// 创建用户
	if err := c.createUserInDB(ctx, &user); err != nil {
		return fmt.Errorf("创建用户失败: %w", err) // 返回错误，让上层根据错误类型决定
	}

	// 设置缓存
	if err := c.setUserCache(ctx, &user); err != nil {
		log.Warnf("用户创建成功但缓存设置失败: username=%s, error=%v", user.Name, err)
		// 缓存失败不影响主流程，不返回错误
	} else {
		log.Infof("用户%s缓存成功", user.Name)
	}

	log.Infof("用户创建成功: username=%s", user.Name)
	return nil
}

// 删除
func (c *UserConsumer) processDeleteOperation(ctx context.Context, msg kafka.Message) error {
	//startTime := time.Now()
	//var operationErr error

	defer func() {
		//	duration := time.Since(startTime).Seconds()
		//metrics.RecordDatabaseQuery("deletet", "users", duration, operationErr)
	}()

	var deleteRequest struct {
		Username  string `json:"username"`
		DeletedAt string `json:"deleted_at"`
	}

	if err := json.Unmarshal(msg.Value, &deleteRequest); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	//	log.Debugf("处理用户删除: username=%s", deleteRequest.Username)

	if err := c.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return c.sendToRetry(ctx, msg, "删除用户失败: "+err.Error())
	}

	if err := c.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("缓存删除失败", "username", deleteRequest.Username, "error", err)
	}

	log.Infof("缓存删除成功: username=%s", deleteRequest.Username)
	return nil
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 这些错误不应该重试，应该发送到死信
	if strings.Contains(errStr, "Duplicate entry") ||
		strings.Contains(errStr, "1062") ||
		strings.Contains(errStr, "23000") ||
		strings.Contains(errStr, "duplicate key value") ||
		strings.Contains(errStr, "23505") ||
		strings.Contains(errStr, "用户已存在") ||
		strings.Contains(errStr, "UserAlreadyExist") ||
		strings.Contains(errStr, "UNMARSHAL_ERROR") {
		return false // ❌ 不可重试错误
	}

	// 这些错误应该重试
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "network error") ||
		strings.Contains(errStr, "database is closed") {
		return true // ✅ 可重试错误
	}

	// 默认情况下重试
	return true
}

func (c *UserConsumer) processUpdateOperation(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("处理用户更新: username=%s", user.Name)

	exists, err := c.checkUserExists(ctx, user.Name)
	if err != nil {
		return c.sendToRetry(ctx, msg, "检查用户存在性失败: "+err.Error())
	}
	if !exists {
		log.Warnf("要更新的用户不存在: username=%s", user.Name)
		return c.sendToDeadLetter(ctx, msg, "USER_NOT_EXISTS")
	}

	if err := c.updateUserInDB(ctx, &user); err != nil {
		return c.sendToRetry(ctx, msg, "更新用户失败: "+err.Error())
	}

	c.setUserCache(ctx, &user)

	log.Infof("用户更新成功: username=%s", user.Name)
	return nil
}

// 数据库操作监控示例
func (c *UserConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	//start := time.Now()
	defer func() {
		//duration := time.Since(start).Seconds()
		//metrics.DatabaseQueryDuration.WithLabelValues("check_exists", "users").Observe(duration)
	}()

	var count int64
	err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ? and status = 1", username).
		Count(&count).Error

	if err != nil {

	}

	return count > 0, err
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

func (c *UserConsumer) deleteUserFromDB(ctx context.Context, username string) error {
	if err := c.db.WithContext(ctx).
		Where("name = ?", username).
		Delete(&v1.User{}).Error; err != nil {
		return fmt.Errorf("数据库删除失败: %v", err)
	}
	return nil
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
