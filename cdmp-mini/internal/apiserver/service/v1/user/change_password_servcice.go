package user

import (
	"context"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) ChangePassword(ctx context.Context, user *v1.User, opt *options.Options) error {

	//判断用户是否存在
	ruser, err := u.checkUserExist(ctx, user.Name, true)
	if err != nil {
		log.Debugf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查: %v", user.Name, err)
	}
	if ruser != nil && ruser.Name == RATE_LIMIT_PREVENTION {
		log.Debugf("用户%s不存在,无法修改密码", user.Name)
		return errors.WithCode(code.ErrUserNotFound, "用户不存在")
	}

	//更新数据库
	_, err = util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
		if err := u.Store.Users().Update(ctx, user, metav1.UpdateOptions{}, opt); err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	//缓存删除
	_, err = util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
		_, delErr := u.Redis.DeleteKey(ctx, strconv.FormatUint(user.ID, 10))
		if delErr != nil {
			log.Debugf("删除用户:%s userid:%v 失败", user.Name, user.ID)
			return nil, delErr
		}
		return nil, nil
	})
	if err != nil {
		return err
	}
	return nil
}
