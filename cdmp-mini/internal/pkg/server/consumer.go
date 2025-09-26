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
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type UserConsumer struct {
	reader   *kafka.Reader
	db       *gorm.DB
	redis    *storage.RedisCluster
	producer *UserProducer
	topic    string
	groupID  string
}

func NewUserConsumer(brokers []string, topic, groupID string, db *gorm.DB, redis *storage.RedisCluster) *UserConsumer {
	// 启动延迟监控

	consumer := &UserConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset,
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
	log.Infof("启动消费者Worker %d, Topic: %s", workerID, c.topic)

	for {
		select {
		case <-ctx.Done():
			log.Infof("Worker %d: 停止消费", workerID)
			return
		default:
			c.processSingleMessage(ctx, workerID)
		}
	}
}

func (c *UserConsumer) processSingleMessage(ctx context.Context, workerID int) {
	startTime := time.Now()
	var operation, messageKey string
	var processingErr error

	// 使用defer统一记录指标
	defer func() {
		c.recordConsumerMetrics(operation, messageKey, startTime, processingErr, workerID)
	}()

	// 从Kafka拉取消息
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		log.Errorf("Worker %d: 获取消息失败: %v", workerID, err)
		processingErr = err
		return
	}

	operation = c.getOperationFromHeaders(msg.Headers)
	messageKey = string(msg.Key)

	// 处理消息
	processingErr = c.processMessage(ctx, msg)

	if processingErr != nil {
		log.Errorf("Worker %d: 处理消息失败: %v", workerID, processingErr)
		return
	}

	// 提交偏移量（确认消费）
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		log.Errorf("Worker %d: 提交偏移量失败: %v", workerID, err)
		metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, "commit", "commit_error").Inc()
	}
}

// recordConsumerMetrics 记录消费者指标
func (c *UserConsumer) recordConsumerMetrics(operation, messageKey string, startTime time.Time, processingErr error, workerID int) {
	totalDuration := time.Since(startTime).Seconds()

	// 记录消息接收（无论成功失败）
	if operation != "" {
		metrics.ConsumerMessagesReceived.WithLabelValues(c.topic, c.groupID, operation).Inc()
	}

	// 如果有错误，记录错误指标
	if processingErr != nil {
		if operation != "" {
			errorType := getErrorType(processingErr)
			metrics.ConsumerProcessingErrors.WithLabelValues(c.topic, c.groupID, operation, errorType).Inc()
			metrics.ConsumerProcessingTime.WithLabelValues(c.topic, c.groupID, operation, "error").Observe(totalDuration)
		}
		return
	}

	// 记录成功处理
	if operation != "" {
		metrics.ConsumerMessagesProcessed.WithLabelValues(c.topic, c.groupID, operation).Inc()
		metrics.ConsumerProcessingTime.WithLabelValues(c.topic, c.groupID, operation, "success").Observe(totalDuration)
	}

	log.Debugf("Worker %d 消息处理完成: topic=%s, key=%s, operation=%s, 总耗时=%.3fs",
		workerID, c.topic, messageKey, operation, totalDuration)
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

// TODO 其他相关的delet,update操作也要加入监控指标
func (c *UserConsumer) processCreateOperation(ctx context.Context, msg kafka.Message) error {
	startTime := time.Now()
	var operationErr error

	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.RecordDatabaseQuery("create", "users", duration, operationErr)
	
	}()

	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		operationErr = fmt.Errorf("unmarshal_error: %w", err)
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("处理用户创建: username=%s", user.Name)

	// 2. 幂等性检查
	exists, err := c.checkUserExists(ctx, user.Name)
	if err != nil {
		operationErr = fmt.Errorf("check_exists_error: %w", err)
		return c.sendToRetry(ctx, msg, "检查用户存在性失败: "+err.Error())
	}
	if exists {
		log.Debugf("用户已存在，跳过创建: username=%s", user.Name)
		return nil
	}

	// 创建用户
	if err := c.createUserInDB(ctx, &user); err != nil {
		operationErr = fmt.Errorf("create_error: %w", err)
		return c.sendToRetry(ctx, msg, "创建用户失败: "+err.Error())
	}

	// 设置缓存
	if err := c.setUserCache(ctx, &user); err != nil {
		log.Warnf("用户创建成功但缓存设置失败: username=%s, error=%v", user.Name, err)
		// 缓存失败不影响主流程，不记录为操作错误
	}

	log.Infof("用户创建成功: username=%s", user.Name)
	return nil
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

// internal/pkg/server/consumer.go
// 修改 processDeleteOperation 方法
func (c *UserConsumer) processDeleteOperation(ctx context.Context, msg kafka.Message) error {
	var deleteRequest struct {
		Username  string `json:"username"`
		DeletedAt string `json:"deleted_at"`
	}

	if err := json.Unmarshal(msg.Value, &deleteRequest); err != nil {
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("处理用户删除: username=%s", deleteRequest.Username)

	exists, err := c.checkUserExists(ctx, deleteRequest.Username)
	if err != nil {
		return c.sendToRetry(ctx, msg, "检查用户存在性失败: "+err.Error())
	}
	if !exists {
		log.Warnf("要删除的用户不存在: username=%s", deleteRequest.Username)
		return nil
	}

	if err := c.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return c.sendToRetry(ctx, msg, "删除用户失败: "+err.Error())
	}

	if err := c.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("缓存删除失败", "username", deleteRequest.Username, "error", err)
	}

	// 布隆过滤器不支持删除操作，只能等待自然过期或者重建
	// 这里记录日志，但不做任何操作
	log.Debugf("用户已删除，布隆过滤器需要等待重建或自然过期: username=%s", deleteRequest.Username)

	log.Infof("用户删除成功: username=%s", deleteRequest.Username)
	return nil
}

// 数据库操作监控示例
func (c *UserConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.DatabaseQueryDuration.WithLabelValues("check_exists", "users").Observe(duration)
	}()

	var count int64
	err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error

	if err != nil {
		metrics.DatabaseQueryErrors.WithLabelValues("check_exists", "users", getErrorType(err)).Inc()
	}

	return count > 0, err
}

func (c *UserConsumer) createUserInDB(ctx context.Context, user *v1.User) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.DatabaseQueryDuration.WithLabelValues("create", "users").Observe(duration)
	}()

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	if err := c.db.WithContext(ctx).Create(user).Error; err != nil {
		metrics.DatabaseQueryErrors.WithLabelValues("create", "users", getErrorType(err)).Inc()
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
	return err
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
