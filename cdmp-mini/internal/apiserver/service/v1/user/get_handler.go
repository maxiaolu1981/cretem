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

func (u *UserService) Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	// 合并所有字段，只声明一次 logger
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"operation", "Get",
	)

	if !u.BloomFilter.TestString(username) {
		logger.Debug("用户名经布隆过滤器验证肯定不存在，直接返回")
		return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在%s", username)
	}

	user, err := u.Store.Users().Get(ctx, username, opts, u.Options)
	if err != nil {
		logger.Debugw("服务层:用户查询失败:", username, "error:", err.Error())
		return nil, err
	}
	return user, nil
}

func (u *UserService) ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error) {
	return nil, nil
}
