package user

import (
	"context"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Get 查询用户（按用户名）- 生产级大并发版本
func (u *Users) Get(ctx context.Context, username string,
	opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {

	log.Debugf("store:开始处理用户%vget请求...", username)
	// 设置包含重试时间预算的超时上下文
	totalCtx, cancel := u.createTimeoutContext(ctx, opt.ServerRunOptions.CtxTimeout, 3)
	defer cancel()

	var resultUser *v1.User

	// 配置生产级重试策略
	queryConfig := db.RetryConfig{
		MaxAttempts:   opt.MysqlOptions.MaxRetryAttempts, // 最多重试2次（总共3次尝试）
		InitialDelay:  opt.MysqlOptions.InitialDelay,     // 初始延迟
		MaxDelay:      opt.MysqlOptions.MaxDelay,         // 大并发下适当增加最大延迟
		BackoffFactor: opt.MysqlOptions.BackoffFactor,
		Jitter:        opt.MysqlOptions.Jitter,
		IsRetryable:   u.isRetryableError,
	}

	// 执行带重试的数据库查询
	err := db.Do(totalCtx, queryConfig, func(attemptCtx context.Context) error {
		attemptStart := time.Now()
		user, innerErr := u.executeSingleGet(attemptCtx, username)
		if innerErr != nil {
			return innerErr
		}
		resultUser = user
		logger.Debugf("单次查询尝试成功%v,%v",
			"attempt_cost_ms", time.Since(attemptStart).Milliseconds())
		return nil
	})

	if err != nil {
		return nil, u.handleGetError(err)
	}
	log.Debugf("查询用户%s成功", username)
	return resultUser, nil
}
