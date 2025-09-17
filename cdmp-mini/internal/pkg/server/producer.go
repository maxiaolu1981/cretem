package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/segmentio/kafka-go"
)

var _ producer.MessageProducer = (*Producer)(nil)

// 常量定义：重试和死信Topic的命名后缀
const (
	RetryTopicSuffix      = ".retry"
	DeadLetterTopicSuffix = ".deadletter"
)

// 生产者
type Producer struct {
	writer          *kafka.Writer // 主Topic生产者
	retryWriter     *kafka.Writer // 重试/死信Topic生产者（注重可靠性）
	mainTopic       string
	retryTopic      string
	deadLetterTopic string
	maxRetries      int // 最大重试次数
}

// 创建生产者
func (g *GenericAPIServer) NewKafkaProducer(brokers []string, mainTopic string, maxRetries int) *Producer {
	// 根据主Topic名称生成重试和死信Topic名称
	retryTopic := mainTopic + RetryTopicSuffix
	deadLetterTopic := mainTopic + DeadLetterTopicSuffix

	// 1. 主Topic Writer (高性能配置)
	mainWriter := &kafka.Writer{
		Addr:            kafka.TCP(brokers...),
		Topic:           mainTopic,
		MaxAttempts:     3,
		WriteBackoffMin: 100 * time.Millisecond,
		WriteBackoffMax: 1 * time.Second,
		BatchBytes:      1048576, // 1MB
		BatchSize:       g.options.KafkaOptions.BatchSize,
		BatchTimeout:    g.options.KafkaOptions.BatchTimeout,
		WriteTimeout:    30 * time.Second,
		Balancer:        &kafka.LeastBytes{},
		Compression:     kafka.Snappy,
		RequiredAcks:    kafka.RequireOne, // 主Topic可以追求速度
		Async:           true,
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				for _, msg := range messages {
					log.Errorf("主消息发送失败! Topic: %s, Key: %s, Error: %v", mainTopic, string(msg.Key), err)
				}
			}
		},
	}

	// 2. 重试/死信Topic Writer (高可靠性配置)
	reliableWriter := &kafka.Writer{
		Addr:            kafka.TCP(brokers...),
		MaxAttempts:     10,
		WriteBackoffMin: 500 * time.Millisecond,
		WriteBackoffMax: 5 * time.Second,
		BatchSize:       1,
		BatchTimeout:    0,
		WriteTimeout:    60 * time.Second,
		RequiredAcks:    kafka.RequireAll,
		Async:           false,
		Compression:     kafka.Snappy,
	}

	return &Producer{
		writer:          mainWriter,
		retryWriter:     reliableWriter,
		mainTopic:       mainTopic,
		retryTopic:      retryTopic,
		deadLetterTopic: deadLetterTopic,
		maxRetries:      maxRetries,
	}
}

func (p *Producer) SendUserCreateMessage(ctx context.Context, user *v1.User) error {
	userData, err := json.Marshal(user)
	if err != nil {
		log.Errorf("消息序列化失败: User: %+v, Error: %v", user, err)
		return errors.WithCode(code.ErrEncodingJSON, "消息序列化失败: %v", err)
	}

	msg := kafka.Message{
		Key:   []byte(user.Name),
		Value: userData,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: "original-timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: "retry-count", Value: []byte("0")},
		},
	}

	log.Infof("准备发送用户消息到主Topic: topic=%s, user=%s", p.mainTopic, user.Name)

	err = p.writer.WriteMessages(ctx, msg)
	if err != nil {
		log.Errorf("主Topic发送失败，尝试发送到重试Topic. Error: %v, User: %s", err, user.Name)

		retryMsg := msg
		retryMsg.Headers = append(retryMsg.Headers, kafka.Header{
			Key:   "first-error",
			Value: []byte(err.Error()),
		})

		retryErr := p.sendToRetryTopic(ctx, retryMsg)
		if retryErr != nil {
			log.Errorf("严重错误：无法发送到重试Topic！主消息已丢失！ User: %s, Error: %v", user.Name, retryErr)
			return errors.WithCode(code.ErrKafkaSendFailed, "无法发送消息到主Topic或重试Topic: %v", retryErr)
		}
		return nil
	}

	log.Infof("用户消息成功发送到主Topic: user=%s", user.Name)
	return nil
}

// sendToRetryTopic 发送消息到重试Topic
func (p *Producer) sendToRetryTopic(ctx context.Context, msg kafka.Message) error {
	p.retryWriter.Topic = p.retryTopic
	defer func() {
		p.retryWriter.Topic = ""
	}()

	log.Infof("发送消息到重试Topic: topic=%s, key=%s", p.retryTopic, string(msg.Key))

	err := p.retryWriter.WriteMessages(ctx, msg)
	if err != nil {
		log.Errorf("发送到重试Topic失败: %v", err)
		return err
	}

	log.Infof("成功发送消息到重试Topic: key=%s", string(msg.Key))
	return nil
}

// SendToRetryTopic 供消费者调用的方法
func (p *Producer) SendToRetryTopic(ctx context.Context, msg kafka.Message) error {
	return p.sendToRetryTopic(ctx, msg)
}

// SendToDeadLetterTopic 发送消息到死信Topic
func (p *Producer) SendToDeadLetterTopic(ctx context.Context, msg kafka.Message, reason string) error {
	deadLetterMsg := msg
	deadLetterMsg.Headers = append(deadLetterMsg.Headers, kafka.Header{
		Key:   "deadletter-reason",
		Value: []byte(reason),
	}, kafka.Header{
		Key:   "deadletter-timestamp",
		Value: []byte(time.Now().Format(time.RFC3339)),
	})

	p.retryWriter.Topic = p.deadLetterTopic
	defer func() {
		p.retryWriter.Topic = ""
	}()

	log.Warnf("发送消息到死信Topic: topic=%s, key=%s, reason=%s", p.deadLetterTopic, string(msg.Key), reason)

	err := p.retryWriter.WriteMessages(ctx, deadLetterMsg)
	if err != nil {
		log.Errorf("严重错误：无法发送到死信Topic！消息将永久丢失！ Key: %s, Error: %v", string(msg.Key), err)
		return err
	}

	log.Warnf("成功发送消息到死信Topic: key=%s", string(msg.Key))
	return nil
}

func (p *Producer) Close() error {
	err1 := p.writer.Close()
	err2 := p.retryWriter.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
