// internal/pkg/server/retry_consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type RetryConsumer struct {
	reader     *kafka.Reader
	db         *gorm.DB
	redis      *storage.RedisCluster
	producer   *UserProducer
	maxRetries int
}

func NewRetryConsumer(brokers []string, db *gorm.DB, redis *storage.RedisCluster, producer *UserProducer) *RetryConsumer {
	return &RetryConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          UserRetryTopic,
			GroupID:        "user-retry-group",
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: 2 * time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		db:         db,
		redis:      redis,
		producer:   producer,
		maxRetries: MaxRetryCount,
	}
}

func (rc *RetryConsumer) Close() error {
	if rc.reader != nil {
		return rc.reader.Close()
	}
	return nil
}

func (rc *RetryConsumer) StartConsuming(ctx context.Context, workerCount int) {
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rc.retryWorker(ctx, workerID)
		}(i)
	}
	wg.Wait()
}

func (rc *RetryConsumer) retryWorker(ctx context.Context, workerID int) {
	log.Infof("启动重试消费者Worker %d", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Infof("重试Worker %d: 停止消费", workerID)
			return
		default:
			msg, err := rc.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("重试Worker %d: 获取消息失败: %v", workerID, err)
				continue
			}

			if err := rc.processRetryMessage(ctx, msg); err != nil {
				log.Errorf("重试Worker %d: 处理消息失败: %v", workerID, err)
				continue
			}

			if err := rc.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("重试Worker %d: 提交偏移量失败: %v", workerID, err)
			}
		}
	}
}

func (rc *RetryConsumer) processRetryMessage(ctx context.Context, msg kafka.Message) error {
	operation := rc.getOperationFromHeaders(msg.Headers)

	switch operation {
	case OperationCreate:
		return rc.processRetryCreate(ctx, msg)
	case OperationUpdate:
		return rc.processRetryUpdate(ctx, msg)
	case OperationDelete:
		return rc.processRetryDelete(ctx, msg)
	default:
		log.Errorf("重试消息未知操作类型: %s", operation)
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "UNKNOWN_OPERATION_IN_RETRY: "+operation)
	}
}

func (rc *RetryConsumer) getOperationFromHeaders(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == HeaderOperation {
			return string(header.Value)
		}
	}
	return OperationCreate
}

func (rc *RetryConsumer) parseRetryHeaders(headers []kafka.Header) (int, time.Time, string) {
	var (
		currentRetryCount int
		nextRetryTime     time.Time
		lastError         string
	)

	for _, header := range headers {
		switch header.Key {
		case HeaderRetryCount:
			if count, err := strconv.Atoi(string(header.Value)); err == nil {
				currentRetryCount = count
			}
		case HeaderNextRetryTS:
			if t, err := time.Parse(time.RFC3339, string(header.Value)); err == nil {
				nextRetryTime = t
			}
		case HeaderRetryError:
			lastError = string(header.Value)
		}
	}

	return currentRetryCount, nextRetryTime, lastError
}

func (rc *RetryConsumer) processRetryCreate(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	log.Infof("处理重试创建: username=%s, 重试次数=%d, 上次错误=%s",
		user.Name, currentRetryCount, lastError)

	if currentRetryCount >= rc.maxRetries {
		log.Warnf("创建消息达到最大重试次数(%d), 发送到死信队列: username=%s",
			rc.maxRetries, user.Name)
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		remaining := time.Until(nextRetryTime)
		log.Debugf("等待重试时间到达: username=%s, 剩余=%v", user.Name, remaining)
		select {
		case <-time.After(remaining):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	log.Infof("开始第%d次重试创建: username=%s", currentRetryCount+1, user.Name)

	exists, err := rc.checkUserExists(ctx, user.Name)
	if err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	}
	if exists {
		log.Debugf("用户已存在，重试创建成功: username=%s", user.Name)
		return nil
	}

	if err := rc.createUserInDB(ctx, &user); err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "创建用户失败: "+err.Error())
	}

	if err := rc.setUserCache(ctx, &user); err != nil {
		log.Errorw("重试创建成功但缓存写入失败", "username", user.Name, "error", err)
	}

	log.Infof("第%d次重试创建成功: username=%s", currentRetryCount+1, user.Name)
	return nil
}

func (rc *RetryConsumer) processRetryUpdate(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	log.Infof("处理重试更新: username=%s, 重试次数=%d", user.Name, currentRetryCount)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		time.Sleep(time.Until(nextRetryTime))
	}

	exists, err := rc.checkUserExists(ctx, user.Name)
	if err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	}
	if !exists {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "USER_NOT_EXISTS_FOR_UPDATE")
	}

	if err := rc.updateUserInDB(ctx, &user); err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "更新用户失败: "+err.Error())
	}

	if err := rc.setUserCache(ctx, &user); err != nil {
		log.Errorw("重试更新成功但缓存写入失败", "username", user.Name, "error", err)
	}

	log.Infof("第%d次重试更新成功: username=%s", currentRetryCount+1, user.Name)
	return nil
}

// internal/pkg/server/retry_consumer.go
// 修改 processRetryDelete 方法
func (rc *RetryConsumer) processRetryDelete(ctx context.Context, msg kafka.Message) error {
	var deleteRequest struct {
		Username  string `json:"username"`
		DeletedAt string `json:"deleted_at"`
	}

	if err := json.Unmarshal(msg.Value, &deleteRequest); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	log.Infof("处理重试删除: username=%s, 重试次数=%d", deleteRequest.Username, currentRetryCount)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		time.Sleep(time.Until(nextRetryTime))
	}

	exists, err := rc.checkUserExists(ctx, deleteRequest.Username)
	if err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	}
	if !exists {
		log.Debugf("用户已不存在，重试删除成功: username=%s", deleteRequest.Username)
		return nil
	}

	if err := rc.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, "删除用户失败: "+err.Error())
	}

	if err := rc.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("重试删除成功但缓存删除失败", "username", deleteRequest.Username, "error", err)
	}

	// 布隆过滤器不支持删除操作
	log.Debugf("重试删除成功，布隆过滤器需要等待重建: username=%s", deleteRequest.Username)

	log.Infof("第%d次重试删除成功: username=%s", currentRetryCount+1, deleteRequest.Username)
	return nil
}

func (rc *RetryConsumer) prepareNextRetry(ctx context.Context, msg kafka.Message, currentRetryCount int, errorInfo string) error {
	newRetryCount := currentRetryCount + 1

	nextRetryDelay := time.Duration(1<<newRetryCount) * BaseRetryDelay
	if nextRetryDelay > MaxRetryDelay {
		nextRetryDelay = MaxRetryDelay
	}

	nextRetryTime := time.Now().Add(nextRetryDelay)

	newHeaders := []kafka.Header{
		{Key: HeaderOperation, Value: []byte(rc.getOperationFromHeaders(msg.Headers))},
		{Key: HeaderOriginalTimestamp, Value: []byte(rc.getOriginalTimestamp(msg.Headers))},
		{Key: HeaderRetryCount, Value: []byte(strconv.Itoa(newRetryCount))},
		{Key: HeaderNextRetryTS, Value: []byte(nextRetryTime.Format(time.RFC3339))},
		{Key: HeaderRetryError, Value: []byte(errorInfo)},
	}

	retryMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: newHeaders,
		Time:    time.Now(),
	}

	log.Warnf("准备第%d次重试: delay=%v, error=%s", newRetryCount, nextRetryDelay, errorInfo)
	return rc.producer.sendToRetryTopic(ctx, retryMsg, errorInfo)
}

func (rc *RetryConsumer) getOriginalTimestamp(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == HeaderOriginalTimestamp {
			return string(header.Value)
		}
	}
	return time.Now().Format(time.RFC3339)
}

func (rc *RetryConsumer) createUserInDB(ctx context.Context, user *v1.User) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	return rc.db.WithContext(ctx).Create(user).Error
}

func (rc *RetryConsumer) updateUserInDB(ctx context.Context, user *v1.User) error {
	user.UpdatedAt = time.Now()
	return rc.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", user.Name).
		Updates(map[string]interface{}{
			"email":      user.Email,
			"password":   user.Password,
			"status":     user.Status,
			"updated_at": user.UpdatedAt,
		}).Error
}

func (rc *RetryConsumer) deleteUserFromDB(ctx context.Context, username string) error {
	return rc.db.WithContext(ctx).
		Where("name = ?", username).
		Delete(&v1.User{}).Error
}

func (rc *RetryConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := rc.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error
	return count > 0, err
}

func (rc *RetryConsumer) setUserCache(ctx context.Context, user *v1.User) error {
	cacheKey := fmt.Sprintf("user:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return rc.redis.SetKey(ctx, cacheKey, string(data), 24*time.Hour)
}

func (rc *RetryConsumer) deleteUserCache(ctx context.Context, username string) error {
	cacheKey := fmt.Sprintf("user:%s", username)
	_, err := rc.redis.DeleteKey(ctx, cacheKey)
	return err
}
