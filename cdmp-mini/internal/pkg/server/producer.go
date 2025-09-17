// internal/pkg/server/producer.go
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

var _ producer.MessageProducer = (*UserProducer)(nil)

type UserProducer struct {
	writer      *kafka.Writer
	retryWriter *kafka.Writer
	maxRetries  int
}

func NewUserProducer(brokers []string, batchSize int, batchTimeout time.Duration) *UserProducer {
	// 主Writer（高性能配置）
	mainWriter := &kafka.Writer{
		Addr:            kafka.TCP(brokers...),
		MaxAttempts:     3,
		WriteBackoffMin: 100 * time.Millisecond,
		WriteBackoffMax: 1 * time.Second,
		BatchBytes:      1048576,
		BatchSize:       batchSize,
		BatchTimeout:    batchTimeout,
		WriteTimeout:    30 * time.Second,
		Balancer:        &kafka.LeastBytes{},
		Compression:     kafka.Snappy,
		RequiredAcks:    kafka.RequireOne,
		Async:           true,
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				for _, msg := range messages {
					log.Errorf("消息发送失败! Key: %s, Error: %v", string(msg.Key), err)
				}
			}
		},
	}

	// 重试Writer（高可靠配置）
	reliableWriter := &kafka.Writer{
		Addr:            kafka.TCP(brokers...),
		MaxAttempts:     10,
		WriteBackoffMin: 500 * time.Millisecond,
		WriteBackoffMax: 5 * time.Second,
		BatchSize:       1,
		WriteTimeout:    60 * time.Second,
		RequiredAcks:    kafka.RequireAll,
		Async:           false,
		Compression:     kafka.Snappy,
	}

	return &UserProducer{
		writer:      mainWriter,
		retryWriter: reliableWriter,
		maxRetries:  MaxRetryCount,
	}
}

func (p *UserProducer) SendUserCreateMessage(ctx context.Context, user *v1.User) error {
	return p.sendUserMessage(ctx, user, OperationCreate, UserCreateTopic)
}

func (p *UserProducer) SendUserUpdateMessage(ctx context.Context, user *v1.User) error {
	return p.sendUserMessage(ctx, user, OperationUpdate, UserUpdateTopic)
}

func (p *UserProducer) SendUserDeleteMessage(ctx context.Context, username string) error {
	deleteData := map[string]interface{}{
		"username":   username,
		"deleted_at": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(deleteData)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "删除消息序列化失败")
	}

	msg := kafka.Message{
		Key:   []byte(username),
		Value: data,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: HeaderOperation, Value: []byte(OperationDelete)},
			{Key: HeaderOriginalTimestamp, Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: HeaderRetryCount, Value: []byte("0")},
		},
	}

	return p.sendWithRetry(ctx, msg, UserDeleteTopic)
}

func (p *UserProducer) sendUserMessage(ctx context.Context, user *v1.User, operation, topic string) error {
	userData, err := json.Marshal(user)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "用户消息序列化失败")
	}

	msg := kafka.Message{
		Key:   []byte(user.Name),
		Value: userData,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: HeaderOperation, Value: []byte(operation)},
			{Key: HeaderOriginalTimestamp, Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: HeaderRetryCount, Value: []byte("0")},
		},
	}

	return p.sendWithRetry(ctx, msg, topic)
}

func (p *UserProducer) sendWithRetry(ctx context.Context, msg kafka.Message, topic string) error {
	p.writer.Topic = topic
	defer func() { p.writer.Topic = "" }()

	err := p.writer.WriteMessages(ctx, msg)
	if err != nil {
		log.Errorf("Topic %s 发送失败，尝试重试Topic. Key: %s", topic, string(msg.Key))
		return p.sendToRetryTopic(ctx, msg, err.Error())
	}

	log.Infof("消息成功发送到Topic %s: key=%s", topic, string(msg.Key))
	return nil
}

func (p *UserProducer) sendToRetryTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	p.retryWriter.Topic = UserRetryTopic
	defer func() { p.retryWriter.Topic = "" }()

	retryMsg := msg
	retryMsg.Headers = append(retryMsg.Headers, kafka.Header{
		Key:   HeaderRetryError,
		Value: []byte(errorInfo),
	})

	err := p.retryWriter.WriteMessages(ctx, retryMsg)
	if err != nil {
		log.Errorf("发送到重试Topic失败: %v", err)
		return err
	}

	log.Infof("消息发送到重试Topic: key=%s", string(msg.Key))
	return nil
}

func (p *UserProducer) SendToDeadLetterTopic(ctx context.Context, msg kafka.Message, reason string) error {
	p.retryWriter.Topic = UserDeadLetterTopic
	defer func() { p.retryWriter.Topic = "" }()

	deadLetterMsg := msg
	deadLetterMsg.Headers = append(deadLetterMsg.Headers,
		kafka.Header{Key: HeaderDeadLetterReason, Value: []byte(reason)},
		kafka.Header{Key: HeaderDeadLetterTS, Value: []byte(time.Now().Format(time.RFC3339))},
	)

	err := p.retryWriter.WriteMessages(ctx, deadLetterMsg)
	if err != nil {
		log.Errorf("发送到死信Topic失败: %v", err)
		return err
	}

	log.Warnf("消息发送到死信Topic: key=%s, reason=%s", string(msg.Key), reason)
	return nil
}

func (p *UserProducer) Close() error {
	if p.writer != nil {
		if err := p.writer.Close(); err != nil {
			return err
		}
	}

	if p.retryWriter != nil {
		if err := p.retryWriter.Close(); err != nil {
			return err
		}
	}

	return nil
}
