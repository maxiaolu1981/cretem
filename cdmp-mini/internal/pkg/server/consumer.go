// internal/pkg/server/consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type UserConsumer struct {
	reader      *kafka.Reader
	db          *gorm.DB
	BloomFilter *bloom.BloomFilter
	BloomMutex  sync.RWMutex
	redis       *storage.RedisCluster
	producer    *UserProducer
	topic       string
}

func NewUserConsumer(brokers []string, topic, groupID string, db *gorm.DB, redis *storage.RedisCluster) *UserConsumer {
	return &UserConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		db:    db,
		redis: redis,
		topic: topic,
	}
}

func (c *UserConsumer) SetProducer(producer *UserProducer) {
	c.producer = producer
}

func (c *UserConsumer) SetBloomFilter(bloomFilter *bloom.BloomFilter) {
	c.BloomFilter = bloomFilter
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
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("Worker %d: 获取消息失败: %v", workerID, err)
				continue
			}

			if err := c.processMessage(ctx, msg); err != nil {
				log.Errorf("Worker %d: 处理消息失败: %v", workerID, err)
				continue
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("Worker %d: 提交偏移量失败: %v", workerID, err)
			}
		}
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
		return c.sendToDeadLetter(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
	}

	log.Debugf("处理用户创建: username=%s", user.Name)

	exists, err := c.checkUserExists(ctx, user.Name)
	if err != nil {
		return c.sendToRetry(ctx, msg, "检查用户存在性失败: "+err.Error())
	}
	if exists {
		log.Debugf("用户已存在，跳过创建: username=%s", user.Name)
		return nil
	}

	if err := c.createUserInDB(ctx, &user); err != nil {
		return c.sendToRetry(ctx, msg, "创建用户失败: "+err.Error())
	}

	if err := c.setUserCache(ctx, &user); err != nil {
		log.Errorw("缓存写入失败", "username", user.Name, "error", err)
	}

	if c.BloomFilter != nil {
		c.BloomFilter.AddString(user.Name)
		log.Debugf("用户添加到布隆过滤器: username=%s", user.Name)
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

	if err := c.setUserCache(ctx, &user); err != nil {
		log.Errorw("缓存更新失败", "username", user.Name, "error", err)
	}

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

func (c *UserConsumer) createUserInDB(ctx context.Context, user *v1.User) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	if err := c.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("数据库创建失败: %v", err)
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

func (c *UserConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error
	return count > 0, err
}

func (c *UserConsumer) setUserCache(ctx context.Context, user *v1.User) error {
	cacheKey := fmt.Sprintf("user:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.redis.SetKey(ctx, cacheKey, string(data), 24*time.Hour)
}

func (c *UserConsumer) deleteUserCache(ctx context.Context, username string) error {
	cacheKey := fmt.Sprintf("user:%s", username)
	_, err := c.redis.DeleteKey(ctx, cacheKey)
	return err
}

func (c *UserConsumer) sendToRetry(ctx context.Context, msg kafka.Message, errorInfo string) error {
	if c.producer == nil {
		return fmt.Errorf("producer未初始化")
	}

	retryMsg := msg
	retryMsg.Headers = append(retryMsg.Headers, kafka.Header{
		Key:   HeaderRetryError,
		Value: []byte(errorInfo),
	})

	return c.producer.sendToRetryTopic(ctx, retryMsg, errorInfo)
}

func (c *UserConsumer) sendToDeadLetter(ctx context.Context, msg kafka.Message, reason string) error {
	if c.producer == nil {
		return fmt.Errorf("producer未初始化")
	}
	return c.producer.SendToDeadLetterTopic(ctx, msg, reason)
}
