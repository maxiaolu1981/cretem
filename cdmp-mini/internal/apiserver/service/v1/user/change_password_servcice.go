package user

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
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

	// Save changed fields.
	if err := u.Store.Users().Update(ctx, user, metav1.UpdateOptions{}, opt); err != nil {
		return errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return nil
}
