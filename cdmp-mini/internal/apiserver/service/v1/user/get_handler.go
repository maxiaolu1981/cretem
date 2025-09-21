package user

import (
	"context"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/bloomfilter"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *UserService) Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	logger := log.L(ctx).WithValues("service", "UserService", "operation", "Get")
	cacheKey := u.generateUserCacheKey(username)

	bloom, err := bloomfilter.GetFilter()
	if err != nil {
		log.Errorf("加载bloom服务失败%v", err)
	}
	// 1. 布隆过滤器检查
	if bloom != nil && !bloom.Test("username", username) {
		bloom.Mu.Lock()
		// 双重检查，避免并发重复添加
		if !bloom.Test("username", username) {
			bloom.Add("username", username)
			go u.cacheNullValue(ctx, cacheKey)
		}
		bloom.Mu.Unlock()
		return nil, nil
	}

	// 2. 使用singleflight包装整个查询
	result, err, _ := u.group.Do(cacheKey, func() (interface{}, error) {
		return u.getUserWithCache(ctx, username, cacheKey, opts, opt)
	})
	if err != nil {
		logger.Debugf("查询失败", "username", username, "error", err.Error())
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*v1.User), nil
}

// 内部方法，不包含singleflight
func (u *UserService) getUserWithCache(ctx context.Context, username, cacheKey string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	logger := log.L(ctx).WithValues("operation", "getUserWithCache")

	// 设置Redis超时
	redisTimeout := u.Options.RedisOptions.Timeout
	if redisTimeout == 0 {
		redisTimeout = 5 * time.Second // 修正超时时间
	}
	redisCtx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()

	// 查询缓存
	cachedUser, isCached, err := u.getFromCache(redisCtx, cacheKey)
	if err != nil {
		logger.Warnw("缓存查询失败，降级到数据库查询", "error", err.Error())
		metrics.CacheHits.WithLabelValues("miss").Inc()
		return u.getUserFromDBAndSetCache(ctx, username, cacheKey, opts, opt)
	}

	if isCached {
		if cachedUser != nil {
			metrics.CacheHits.WithLabelValues("hit").Inc()
			return cachedUser, nil
		}
		metrics.CacheHits.WithLabelValues("null_hit").Inc()
		return nil, nil
	}

	// 缓存中没有记录
	logger.Debugw("缓存未命中，查询数据库")
	metrics.CacheHits.WithLabelValues("no_record").Inc()
	return u.getUserFromDBAndSetCache(ctx, username, cacheKey, opts, opt)
}
