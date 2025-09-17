package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type Consumer struct {
	reader      *kafka.Reader
	db          *gorm.DB
	BloomFilter *bloom.BloomFilter
	BloomMutex  sync.RWMutex
	redis       *storage.RedisCluster
	producer    *Producer // 新增字段，用于发送到重试队列
}

func NewKafkaConsumer(brokers []string, topic,
	groupID string, db *gorm.DB, redis *storage.RedisCluster) *Consumer {
	return &Consumer{
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
	}
}

// SetProducer 设置生产者实例
func (c *Consumer) SetProducer(producer *Producer) {
	c.producer = producer
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}

func (c *Consumer) StartConsuming(ctx context.Context, workerCount int) {
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

func (c *Consumer) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			log.Infof("Worker %d: 收到停止信号，退出消费循环", workerID)
			return
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("Worker %d: failed to fetch message: %v", workerID, err)
				continue
			}

			if err := c.processMessage(ctx, msg); err != nil {
				log.Errorf("Worker %d: failed to process message: %v", workerID, err)
				continue
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("Worker %d: failed to commit message: %v", workerID, err)
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		log.Errorf("消息格式错误，无法解析，发送到死信队列. Error: %v, Message: %s", err, string(msg.Value))

		if c.producer != nil {
			deadLetterErr := c.producer.SendToDeadLetterTopic(ctx, msg, "UNMARSHAL_ERROR: "+err.Error())
			if deadLetterErr != nil {
				log.Errorf("严重：无法发送到死信队列！消息丢失！ Error: %v", deadLetterErr)
			}
		}
		return nil
	}

	log.Debugf("开始处理用户创建消息: username=%s, email=%s", user.Name, user.Email)

	// 执行业务逻辑
	err := c.createUserInDB(ctx, &user)
	if err != nil {
		log.Errorf("处理主消息失败，准备发送到重试Topic. Username: %s, Error: %v", user.Name, err)

		if c.producer != nil {
			retryErr := c.sendToRetryTopic(ctx, msg, err.Error())
			if retryErr != nil {
				log.Errorf("严重：无法发送到重试Topic！主消息处理失败且无法重试！ Username: %s, Error: %v", user.Name, retryErr)
				return retryErr
			}
		}
		return nil
	}

	// 成功，写入缓存
	if err := c.setUserCache(ctx, &user); err != nil {
		log.Errorw("缓存写入失败（不影响主流程）", "username", user.Name, "error", err)
	}
	log.Infof("成功创建用户: username=%s, id=%d", user.Name, user.ID)
	return nil
}

// sendToRetryTopic 发送消息到重试Topic
func (c *Consumer) sendToRetryTopic(ctx context.Context, originalMsg kafka.Message, errorInfo string) error {
	retryMsg := originalMsg

	// 更新重试次数
	currentRetryCount := 0
	for _, header := range originalMsg.Headers {
		if header.Key == "retry-count" {
			if count, err := strconv.Atoi(string(header.Value)); err == nil {
				currentRetryCount = count
			}
			break
		}
	}

	newRetryCount := currentRetryCount + 1
	nextRetryTime := time.Now().Add(time.Duration(1<<newRetryCount) * 10 * time.Second) // 指数退避

	// 更新Headers
	retryMsg.Headers = []kafka.Header{
		{Key: "retry-error", Value: []byte(errorInfo)},
		{Key: "retry-count", Value: []byte(strconv.Itoa(newRetryCount))},
		{Key: "next-retry-ts", Value: []byte(nextRetryTime.Format(time.RFC3339))},
	}

	return c.producer.SendToRetryTopic(ctx, retryMsg)
}

// createUserInDB 抽离的用户创建逻辑
func (c *Consumer) createUserInDB(ctx context.Context, user *v1.User) error {
	// 检查用户是否已存在
	exists, err := c.checkUserExists(ctx, user.Name)
	if err != nil {
		return fmt.Errorf("检查用户存在性失败: %v", err)
	}
	if exists {
		log.Debugf("用户已存在，跳过插入: username=%s", user.Name)
		return nil
	}

	// 设置时间字段
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// 插入数据库
	if err := c.db.WithContext(ctx).Create(user).Error; err != nil {
		log.Errorf("创建用户失败: username=%s, error=%v", user.Name, err)
		return fmt.Errorf("创建用户失败: %v", err)
	}

	// 添加到布隆过滤器
	if c.BloomFilter != nil {
		c.BloomFilter.AddString(user.Name)
		log.Debugf("用户已添加到布隆过滤器: username=%s", user.Name)
	}

	return nil
}

// checkUserExists 用户存在性检查
func (c *Consumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	if c.BloomFilter != nil {
		if !c.BloomFilter.TestString(username) {
			return false, nil
		}
	}

	var count int64
	err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("数据库查询失败: %v", err)
	}

	return count > 0, nil
}

// setUserCache 设置用户缓存
func (c *Consumer) setUserCache(ctx context.Context, user *v1.User) error {
	cacheKey := fmt.Sprintf("user:v1:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("用户数据序列化失败: %v", err)
	}

	expireTime := 24 * time.Hour
	if err := c.redis.SetKey(ctx, cacheKey, string(data), expireTime); err != nil {
		return fmt.Errorf("redis设置缓存失败: %v", err)
	}

	log.Debugw("用户缓存写入成功", "username", user.Name)
	return nil
}
