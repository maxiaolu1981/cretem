package user

import (
	"context"
	"time"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Get 查询用户（按用户名）
func (u *Users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	logger := u.getLogger(ctx)
	logger.Debug("存储层:开始查询用户信息")

	// 设置超时上下文
	dbCtx, cancel := u.createTimeoutContext(ctx)
	defer cancel()

	startTime := time.Now()

	var user *v1.User
	var err error

	// 使用自定义配置的retry（查询操作重试延迟更短）
	queryConfig := db.RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 50 * time.Millisecond,
		IsRetryable:  u.isRetryableError, // 使用自定义的重试判断
	}

	err = db.Do(dbCtx, queryConfig, func() error {
		user, err = u.executeSingleGet(dbCtx, username)
		return err
	})

	costMs := time.Since(startTime)
	//log.Warnf("stroe:err:%+v", err)
	if err != nil {
		return nil, u.handleGetError(err, username, logger, costMs)
	}

	u.logGetSuccess(user, logger, costMs)
	return user, nil
}
