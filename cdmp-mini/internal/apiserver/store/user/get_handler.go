package user

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Get 查询用户（按用户名）- 生产级大并发版本
func (u *Users) Get(ctx context.Context, username string,
	opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {

	start := time.Now()
	logger.Debugf("开始查询用户", "username", username)
	// 设置包含重试时间预算的超时上下文
	totalCtx, cancel := u.createTimeoutContext(ctx, opt.ServerRunOptions.CtxTimeout, 3)
	defer cancel()

	var resultUser *v1.User

	// 配置生产级重试策略
	queryConfig := db.RetryConfig{
		MaxAttempts:   2, // 最多重试2次（总共3次尝试）
		InitialDelay:  50 * time.Millisecond,
		MaxDelay:      500 * time.Millisecond, // 大并发下适当增加最大延迟
		BackoffFactor: 2.0,
		Jitter:        true,
		IsRetryable:   u.isRetryableError,
	}

	// 执行带重试的数据库查询
	err := db.Do(totalCtx, queryConfig, func(attemptCtx context.Context) error {
		attemptStart := time.Now()

		user, innerErr := u.executeSingleGet(attemptCtx, username)
		if innerErr != nil {
			logger.Debugf("单次查询尝试失败",
				"attempt_cost_ms", time.Since(attemptStart).Milliseconds(),
				"error", innerErr.Error())
			return innerErr
		}

		resultUser = user
		logger.Debugf("单次查询尝试成功",
			"attempt_cost_ms", time.Since(attemptStart).Milliseconds())
		return nil
	})

	totalCost := time.Since(start)

	if err != nil {
		logger.Warnf("用户查询失败",
			"username", username,
			"total_cost_ms", totalCost.Milliseconds(),
			"error", err.Error(),
			"error_type", fmt.Sprintf("%T", err)) // 记录错误类型
		return nil, u.handleGetError(err, totalCost)
	}

	logger.Infof("用户查询成功",
		"total_cost_ms", totalCost.Milliseconds())
	return resultUser, nil
}
