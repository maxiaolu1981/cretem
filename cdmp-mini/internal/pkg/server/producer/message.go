package producer

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
)

// MessageProducer 定义通用的消息生产者接口
type MessageProducer interface {
	//	SendMessage(ctx context.Context, topic string, value []byte) error
	SendUserCreateMessage(ctx context.Context, user *v1.User) error
	Close() error
}
