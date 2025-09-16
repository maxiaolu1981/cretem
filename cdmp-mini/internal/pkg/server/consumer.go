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
				fmt.Printf("Worker %d: failed to fetch message: %v\n", workerID, err)
				continue
			}

			if err := c.processMessage(ctx, msg); err != nil {
				fmt.Printf("Worker %d: failed to process message: %v\n", workerID, err)
				// 可以考虑重试逻辑或者将失败消息发送到死信队列
				continue
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				fmt.Printf("Worker %d: failed to commit message: %v\n", workerID, err)
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	var user *v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return fmt.Errorf("failed to unmarshal user: %v", err)
	}

	// 新增：使用布隆过滤器快速检查用户名是否存在
	if c.BloomFilter.TestString(user.Name) {
		// 布隆过滤器确认用户名肯定不存在
		log.Debugf("用户名已经存在（布隆过滤器验证）: username=%s", user.Name)
		return nil
	}

	// 设置创建时间和更新时间
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	// 保存用户到数据库
	if err := c.db.Create(&user).Error; err != nil {
		return fmt.Errorf("创建用户失败")
	}
	log.Debugf("成功创建用户:%s\n", user.Name)
	return nil
}
