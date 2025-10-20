package user

import (
	"context"
	"fmt"

	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/trace"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions, opt *options.Options) error {
	//检查用户是否存在

	//判断用户是否存在
	for _, name := range username {
		ruser, err := u.checkUserExist(ctx, name, true)
		if err != nil || ruser == nil || ruser.Name == RATE_LIMIT_PREVENTION || ruser.Name == BLACKLIST_SENTINEL {
			continue
		} else {
			u.Delete(ctx, name, true, opts, opt)
		}
	}
	return nil
}

func (u *UserService) Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions, opt *options.Options) (err error) {
	ctx, span := trace.StartSpan(ctx, "user-service", "delete")
	trace.AddRequestTag(ctx, "username", username)
	businessCode := strconv.Itoa(code.ErrSuccess)
	spanStatus := "success"
	defer func() {
		if err != nil {
			spanStatus = "error"
			if c := errors.GetCode(err); c != 0 {
				businessCode = strconv.Itoa(c)
			} else {
				businessCode = strconv.Itoa(code.ErrUnknown)
			}
		}
		trace.EndSpan(span, spanStatus, businessCode, map[string]interface{}{
			"username": username,
			"force":    force,
		})
	}()

	//检查用户是否存在
	checkCtx, checkSpan := trace.StartSpan(ctx, "user-service", "check_user_exist")
	ruser, existErr := u.checkUserExist(checkCtx, username, true)
	spanStatusCheck := "success"
	spanCodeCheck := strconv.Itoa(code.ErrSuccess)
	if existErr != nil {
		log.Warnf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查: %v", username, existErr)
		spanStatusCheck = "error"
		if c := errors.GetCode(existErr); c != 0 {
			spanCodeCheck = strconv.Itoa(c)
		} else {
			spanCodeCheck = strconv.Itoa(code.ErrUnknown)
		}
	}
	if ruser != nil && (ruser.Name == RATE_LIMIT_PREVENTION || ruser.Name == BLACKLIST_SENTINEL) {
		err = errors.WithCode(code.ErrUserNotFound, "用户不存在,无法删除")
		spanStatusCheck = "error"
		spanCodeCheck = strconv.Itoa(code.ErrUserNotFound)
	}
	trace.EndSpan(checkSpan, spanStatusCheck, spanCodeCheck, map[string]interface{}{
		"username": username,
	})
	if err != nil {
		return err
	}
	if existErr != nil {
		// 继续执行，但将诊断信息写入trace
		trace.AddRequestTag(ctx, "check_exist_error", existErr.Error())
	}

	//物理删除
	if force {
		if u.Producer == nil {
			log.Errorf("生产者转换错误")
			err = errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
			return err
		}
		if u.Producer == nil {
			return fmt.Errorf("producer未初始化")
		}
		// 发送到Kafka
		sendCtx, sendSpan := trace.StartSpan(ctx, "user-service", "producer_send_delete")
		trace.AddRequestTag(sendCtx, "username", username)
		sendErr := u.Producer.SendUserDeleteMessage(sendCtx, username)
		sendStatus := "success"
		sendCode := strconv.Itoa(code.ErrSuccess)
		if sendErr != nil {
			log.Errorf("requestID=%s: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), username, sendErr)
			sendStatus = "error"
			if c := errors.GetCode(sendErr); c != 0 {
				sendCode = strconv.Itoa(c)
			} else {
				sendCode = strconv.Itoa(code.ErrUnknown)
			}
			err = errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
		}
		trace.EndSpan(sendSpan, sendStatus, sendCode, map[string]interface{}{
			"username": username,
		})
		if sendErr != nil {
			return err
		}
		// 记录业务成功

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
