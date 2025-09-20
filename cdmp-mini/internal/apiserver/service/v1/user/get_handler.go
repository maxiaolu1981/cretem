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
	// 1. 生成缓存key
	cacheKey := u.generateUserCacheKey(username)

	// 2. 布隆过滤器检查（快速路径）
	if u.BloomFilter != nil && !u.BloomFilter.TestString(username) {
		// 缓存空值，防止后续请求
		u.cacheNullValue(ctx, cacheKey)
		err := errors.WithCode(code.ErrUserNotFound, "用户不存在%s", username)
		return nil, err
	}

	// 3. 查询缓存
	cachedUser, err := u.getFromCache(ctx, cacheKey)
	if err != nil {
		logger.Warnw("缓存查询失败，降级到数据库查询", "error", err.Error())
	} else {
		// 缓存命中
		if cachedUser != nil {
			return cachedUser, nil
		}
		// 缓存中是空值（用户不存在）
		return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在%s", username)
	}

	//3.使用singleflight防止缓存击穿
	result, err, _ := u.group.Do(cacheKey, func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, u.Options.ServerRunOptions.CtxTimeout)
		defer cancel()
		return u.getUserFromDBAndSetCache(ctx, username, cacheKey, opts, opt)
	})
	if err != nil {
		logger.Errorw("数据库查询失败", "username", username, "error", err.Error())
		return nil, err
	}

	return result.(*v1.User), nil
}
