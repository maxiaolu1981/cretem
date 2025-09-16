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

// 生产者
type Producer struct {
	writer *kafka.Writer
}

// 创建生成者
func NewKafkaProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			Async:        true, // 启用异步模式提高吞吐量
			Completion: func(messages []kafka.Message, err error) {
				if err != nil {
					log.Warnf("消息%v发送失败:v", messages, err)
				}
			},
		},
	}
}

func (p *Producer) SendUserCreateMessage(ctx context.Context, user *v1.User) error {
	userData, err := json.Marshal(user)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "消息序列化失败%v", err)
	}

	msg := kafka.Message{
		Key:   []byte(user.Name), // 使用用户名作为key保证相同邮箱的消息到同一个分区
		Value: userData,
		Time:  time.Now(),
	}

	return p.writer.WriteMessages(ctx, msg)
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
