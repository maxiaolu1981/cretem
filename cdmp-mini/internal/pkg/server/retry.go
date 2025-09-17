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

// 需要新增的 RetryConsumer 结构（可以放在同一个文件或新建文件）
type RetryConsumer struct {
	reader     *kafka.Reader
	db         *gorm.DB
	redis      *storage.RedisCluster
	producer   *Producer
	maxRetries int
}

func NewRetryConsumer(brokers []string, mainTopic, groupID string, db *gorm.DB, redis *storage.RedisCluster, producer *Producer, maxRetries int) *RetryConsumer {
	retryTopic := mainTopic + RetryTopicSuffix

	return &RetryConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          retryTopic,
			GroupID:        groupID,
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
		}),
		db:         db,
		redis:      redis,
		producer:   producer,
		maxRetries: maxRetries,
	}
}

func (rc *RetryConsumer) StartConsuming(ctx context.Context, redis *storage.RedisCluster, workerCount int) {
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := rc.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("Retry Worker %d: failed to fetch message: %v", workerID, err)
				continue
			}

			if err := rc.processRetryMessage(ctx, msg); err != nil {
				log.Errorf("Retry Worker %d: failed to process message: %v", workerID, err)
				continue
			}

			if err := rc.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("Retry Worker %d: failed to commit message: %v", workerID, err)
			}
		}
	}
}

func (rc *RetryConsumer) processRetryMessage(ctx context.Context, msg kafka.Message) error {
	// 1. 解析消息体
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		log.Errorf("重试消息格式错误，发送到死信队列. Error: %v, Message: %s", err, string(msg.Value))
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	// 2. 解析重试相关的Header信息
	var (
		currentRetryCount int
		nextRetryTime     time.Time
		lastError         string
	)

	for _, header := range msg.Headers {
		switch header.Key {
		case "retry-count":
			if count, err := strconv.Atoi(string(header.Value)); err == nil {
				currentRetryCount = count
			}
		case "next-retry-ts":
			if t, err := time.Parse(time.RFC3339, string(header.Value)); err == nil {
				nextRetryTime = t
			}
		case "retry-error":
			lastError = string(header.Value)
		}
	}

	log.Infof("处理重试消息: username=%s, 当前重试次数=%d, 计划重试时间=%v",
		user.Name, currentRetryCount, nextRetryTime)

	// 3. 检查是否达到最大重试次数
	if currentRetryCount >= rc.maxRetries {
		log.Warnf("消息已达到最大重试次数(%d)，发送到死信队列. Username: %s, 最后错误: %s",
			rc.maxRetries, user.Name, lastError)
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	// 4. 检查是否到了重试时间（实现延迟重试）
	now := time.Now()
	if now.Before(nextRetryTime) {
		// 还没到重试时间，计算剩余等待时间
		remaining := time.Until(nextRetryTime)
		log.Debugf("消息未到重试时间，等待 %v 后重试. Username: %s", remaining, user.Name)

		// 可以选择等待或者直接返回（让消息留在队列中稍后重新消费）
		select {
		case <-time.After(remaining):
			// 等待到期后继续处理
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 5. 执行重试逻辑
	log.Infof("开始第%d次重试: username=%s", currentRetryCount+1, user.Name)

	// 执行业务逻辑（与主消费者相同的逻辑）
	err := rc.createUserInDB(ctx, &user)
	if err != nil {
		log.Errorf("第%d次重试失败: username=%s, error=%v", currentRetryCount+1, user.Name, err)

		// 6. 准备下一次重试（指数退避策略）
		newRetryCount := currentRetryCount + 1
		baseDelay := 10 * time.Second
		nextRetryDelay := time.Duration(1<<newRetryCount) * baseDelay // 指数退避：10s, 20s, 40s, 80s...
		newNextRetryTime := now.Add(nextRetryDelay)

		// 7. 更新Header信息
		newHeaders := []kafka.Header{
			{Key: "retry-count", Value: []byte(strconv.Itoa(newRetryCount))},
			{Key: "next-retry-ts", Value: []byte(newNextRetryTime.Format(time.RFC3339))},
			{Key: "retry-error", Value: []byte(err.Error())},
			{Key: "last-retry-timestamp", Value: []byte(now.Format(time.RFC3339))},
		}

		// 保留原始的一些Header（如original-timestamp）
		for _, header := range msg.Headers {
			if header.Key == "original-timestamp" {
				newHeaders = append(newHeaders, header)
				break
			}
		}

		// 8. 重新发送到重试Topic
		retryMsg := kafka.Message{
			Key:     msg.Key,
			Value:   msg.Value,
			Headers: newHeaders,
			Time:    now,
		}

		log.Warnf("重试失败，%v后进行第%d次重试. Username: %s", nextRetryDelay, newRetryCount+1, user.Name)
		return rc.producer.SendToRetryTopic(ctx, retryMsg)
	}

	// 9. 重试成功！
	log.Infof("第%d次重试成功: username=%s", currentRetryCount+1, user.Name)

	// 写入缓存
	if err := rc.setUserCache(ctx, &user); err != nil {
		log.Errorw("重试成功但缓存写入失败", "username", user.Name, "error", err)
	}

	return nil
}

// createUserInDB 重试消费者的数据库操作（与主消费者相同）
func (rc *RetryConsumer) createUserInDB(ctx context.Context, user *v1.User) error {
	exists, err := rc.checkUserExists(ctx, user.Name)
	if err != nil {
		return fmt.Errorf("检查用户存在性失败: %v", err)
	}
	if exists {
		log.Debugf("用户已存在，跳过插入: username=%s", user.Name)
		return nil
	}

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	if err := rc.db.WithContext(ctx).Create(user).Error; err != nil {
		log.Errorf("创建用户失败: username=%s, error=%v", user.Name, err)
		return fmt.Errorf("创建用户失败: %v", err)
	}

	return nil
}

// checkUserExists 用户存在性检查
func (rc *RetryConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := rc.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("数据库查询失败: %v", err)
	}

	return count > 0, nil
}

// setUserCache 设置用户缓存
func (rc *RetryConsumer) setUserCache(ctx context.Context, user *v1.User) error {
	cacheKey := fmt.Sprintf("user:v1:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("用户数据序列化失败: %v", err)
	}

	expireTime := 24 * time.Hour
	if err := rc.redis.SetKey(ctx, cacheKey, string(data), expireTime); err != nil {
		return fmt.Errorf("Redis设置缓存失败: %v", err)
	}

	log.Debugw("用户缓存写入成功", "username", user.Name)
	return nil
}
