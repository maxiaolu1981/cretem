package user

import (
	"context"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Get 查询用户（按用户名）
func (u *Users) Get(ctx context.Context, username string,
	opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	// 设置超时上下文
	dbCtx, cancel := u.createTimeoutContext(ctx)
	defer cancel()
	var resultUser *v1.User
	// 使用自定义配置的retry（查询操作重试延迟更短）
	queryConfig := db.RetryConfig{
		MaxAttempts:  2,                      // 修正：例如，最多重试2次
		InitialDelay: 50 * time.Millisecond,  //每次重试尝试之间等待的固定间隔时间
		MaxBackoff:   100 * time.Millisecond, //最大延迟，避免延迟过长
		IsRetryable:  u.isRetryableError,     //判断
	}

	err := db.Do(dbCtx, queryConfig, func() error {
		user, innerErr := u.executeSingleGet(dbCtx, username)
		if innerErr != nil {
			return innerErr
		}
		resultUser = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resultUser, nil
}
