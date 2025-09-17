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

	//1.布隆过滤器检查（快速路径）
	if !u.BloomFilter.TestString(username) {
		logger.Debug("用户名经布隆过滤器验证肯定不存在，直接返回")
		return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在%s", username)
	}

	cacheKey := u.generateUserCacheKey(username)

	//2.生产缓存key
	cachedUser, err := u.getFromCache(ctx, cacheKey)
	if err != nil {
		logger.Warnw("缓存查询失败，降级到数据库查询", "error", err.Error())
	} else if cachedUser != nil {
		logger.Debug("从缓存命中用户数据")
		return cachedUser, nil
	}

	//3.使用singleflight防止缓存击穿
	result, err, shared := u.group.Do(cacheKey, func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, u.Options.ServerRunOptions.CtxTimeout)
		defer cancel()
		return u.getUserWithCache(ctx, username, cacheKey, opts, opt)
	})

	if err != nil {
		return nil, err
	}

	if shared {
		logger.Debug("singleflight合并了并发请求")
	}

	return result.(*v1.User), nil
}
