package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

const (
	UserServiceCreateTopic = "user-service.create.v1"
	UserServiceUpdateTopic = "user-service.update.v1"
)

type Consumer struct {
	reader      *kafka.Reader
	db          *gorm.DB
	BloomFilter *bloom.BloomFilter
	BloomMutex  sync.RWMutex
}

func NewKafkaConsumer(brokers []string, topic, groupID string, db *gorm.DB) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       10e3, // 10KB
			MaxBytes:       10e6, // 10MB
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		db: db,
	}
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
			return
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("Worker %d: failed to fetch message: %v\n", workerID, err)
				continue
			}

			if err := c.processMessage(ctx, msg); err != nil {
				log.Errorf("Worker %d: failed to process message: %v\n", workerID, err)
				// 可以考虑重试逻辑或者将失败消息发送到死信队列
				continue
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("Worker %d: failed to commit message: %v\n", workerID, err)
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	// 修复1：正确的Unmarshal方式
	var user v1.User // 改为值类型
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return fmt.Errorf("failed to unmarshal user: %v", err)
	}

	// 修复2：添加上下文日志
	log.Debugf("开始处理用户创建消息: username=%s, email=%s", user.Name, user.Email)

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
	if err := c.db.WithContext(ctx).Create(&user).Error; err != nil {
		log.Errorf("创建用户失败: username=%s, error=%v", user.Name, err)
		return fmt.Errorf("创建用户失败: %v", err)
	}

	// 修复3：添加到布隆过滤器（如果存在）
	if c.BloomFilter != nil {
		c.BloomFilter.AddString(user.Name)
		log.Debugf("用户已添加到布隆过滤器: username=%s", user.Name)
	}

	log.Infof("成功创建用户: username=%s, id=%d", user.Name, user.ID)
	return nil
}

// checkUserExists 抽离用户存在性检查逻辑
func (c *Consumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	// 1. 先检查布隆过滤器（如果可用）
	if c.BloomFilter != nil {
		if !c.BloomFilter.TestString(username) {
			// 布隆过滤器确认不存在
			return false, nil
		}
		// 布隆过滤器返回true，需要进一步检查数据库
	}

	// 2. 检查数据库
	var count int64
	err := c.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("数据库查询失败: %v", err)
	}

	return count > 0, nil
}
