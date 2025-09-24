package user

import (
	"context"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {

	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Create",
	)

	//判断用户是否存在
	rUser, err := u.Store.Users().Get(ctx, user.Name, metav1.GetOptions{}, u.Options)
	if err == nil && rUser != nil {
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在%s", user.Name)
	}

	if u.Producer == nil {
		log.Errorf("生产者转换错误")
		return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
	}
	if u.Producer == nil {
		return fmt.Errorf("producer未初始化")
	}
	// 发送到Kafka
	err = u.Producer.SendUserCreateMessage(ctx, user)
	if err != nil {
		log.Errorf("requestID=%s: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), user.Name, err)
		metrics.BusinessFailures.WithLabelValues("user_create", "kafka_send_failed").Inc()
		return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
	}
	// 记录业务成功
	metrics.BusinessSuccess.WithLabelValues("user_create").Inc()
	logger.Infow("用户创建请求已发送到Kafka", "username", user.Name)
	return nil
}
