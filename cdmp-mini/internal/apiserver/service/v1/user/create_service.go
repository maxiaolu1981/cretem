package user

import (
	"context"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {

	log.Debugf("service:开始处理用户%v创建请求...", user.Name)

	//判断用户是否存在
	ruser, err := u.checkUserExist(ctx, user.Name, false)
	if err != nil {
		log.Debugf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查, 放行该用户: %v", user.Name, err)
	}
	if ruser != nil && ruser.Name != RATE_LIMIT_PREVENTION {
		log.Debugf("用户%s已经存在,无法创建", user.Name)
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在")
	}

	if u.Producer == nil {
		log.Errorf("生产者转换错误")
		return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
	}
	// 发送到Kafka
	errKafka := u.Producer.SendUserCreateMessage(ctx, user)
	if errKafka != nil {
		log.Errorf("requestID=%v: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), user.Name, errKafka)
		return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
	}
	log.Debugw("用户创建请求已发送到Kafka", "username", user.Name)

	return nil
}
