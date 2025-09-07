package user

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	// 合并所有字段，只声明一次 logger
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"operation", "Get",
	)
	logger.Debugf("服务层:开始执行用户查询逻辑")

	user, err := u.Store.Users().Get(ctx, username, opts)
	if err != nil {

		coder := errors.ParseCoderByErr(err)
		log.Debugf("service:返回的业务码%v", coder.Code())

		logger.Debugw("服务层:用户查询失败:", username, "error:", err.Error())
		return nil, err
	}
	return user, nil
}

func (u *UserService) ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	return nil, nil
}
