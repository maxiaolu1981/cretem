package user

import (
	"context"
	"fmt"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions, opt *options.Options) error {
	//检查用户是否存在

	//判断用户是否存在
	for _, name := range username {
		ruser, err := u.checkUserExist(ctx, name, true)
		if err != nil || ruser == nil || ruser.Name == RATE_LIMIT_PREVENTION {
			continue
		} else {
			u.Delete(ctx, name, true, opts, opt)
		}
	}
	return nil
}

func (u *UserService) Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions, opt *options.Options) error {

	//检查用户是否存在
	ruser, err := u.checkUserExist(ctx, username, true)
	log.Debugf("ruser=%v, err=%v", ruser, err)
	if err != nil {
		log.Debugf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查: %v", username, err)
	}
	if ruser != nil && ruser.Name == RATE_LIMIT_PREVENTION {
		log.Debugf("用户%s不存在,无法删除", username)
		return errors.WithCode(code.ErrUserNotFound, "用户不存在,无法删除")
	}

	//物理删除
	if force {
		if u.Producer == nil {
			log.Errorf("生产者转换错误")
			return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
		}
		if u.Producer == nil {
			return fmt.Errorf("producer未初始化")
		}
		// 发送到Kafka
		err := u.Producer.SendUserDeleteMessage(ctx, username)
		if err != nil {
			log.Errorf("requestID=%s: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), username, err)
			return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
		}
		// 记录业务成功
		log.Debugw("用户删除请求已发送到Kafka", "username", username)
		return nil

	} else { //更新操作
		// 	opts = metav1.DeleteOptions{Unscoped: false}
		// 	err = u.Store.Users().Delete(ctx, username, opts, u.Options)
		// }
		// if err != nil {
		// 	log.Errorw("用户删除失败", "username", username, "error", err)
		// 	return err
	}

	return nil
}
