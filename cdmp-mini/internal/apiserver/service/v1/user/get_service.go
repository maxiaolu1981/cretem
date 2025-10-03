package user

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *UserService) Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	cacheKey := u.generateUserCacheKey(username)

	// 先尝试无锁查询缓存（大部分请求应该在这里返回）
	user, found, err := u.tryGetFromCache(ctx, cacheKey)
	if err != nil {
		// 缓存查询错误，记录但继续流程
		log.Errorf("缓存查询异常，继续流程", "error", err.Error(), "username", username)
		// 使用 WithLabelValues 来记录错误
		metrics.CacheErrors.WithLabelValues("query_failed", "get").Inc()
	}
	// 缓存命中，直接返回
	if found {
		return user, nil
	}

	// 缓存未命中，使用singleflight保护数据库查询
	result, err, shared := u.group.Do(cacheKey, func() (interface{}, error) {
		return u.getUserFromDBAndSetCache(ctx, username, cacheKey)
	})
	if shared {
		log.Infow("数据库查询被合并，共享结果", "username", username)
		metrics.RequestsMerged.WithLabelValues("get").Inc()
	}
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	return result.(*v1.User), nil
}

// 专门处理缓存查询，不包含降级逻辑
func (u *UserService) tryGetFromCache(ctx context.Context, cacheKey string) (*v1.User, bool, error) {
	redisTimeout := u.Options.RedisOptions.Timeout
	if redisTimeout == 0 {
		redisTimeout = 5 * u.Options.RedisOptions.Timeout
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()

	cachedUser, isCached, err := u.getFromCache(redisCtx, cacheKey)
	if err != nil {
		u.recordCacheError(err, "get_from_cache")
		// 只返回错误，不处理降级
		return nil, false, err
	}

	if isCached {
		if cachedUser != nil {
			metrics.CacheHits.WithLabelValues("hit").Inc()
			return cachedUser, true, nil
		}
		metrics.CacheHits.WithLabelValues("null_hit").Inc()
		return nil, true, nil // 空值缓存命中
	}

	// 缓存中没有记录
	metrics.CacheHits.WithLabelValues("no_record").Inc()
	return nil, false, nil
}

// 记录缓存错误的辅助方法
func (u *UserService) recordCacheError(err error, operation string) {
	errorType := "unknown"

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		errorType = "timeout"
	case errors.Is(err, context.Canceled):
		errorType = "cancelled"
	case errors.Is(err, redis.Nil):
		errorType = "key_not_found"
		return // key不存在是正常情况，不记录为错误
	default:
		// 检查是否是网络错误
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				errorType = "network_timeout"
			} else {
				errorType = "network_error"
			}
		} else if strings.Contains(err.Error(), "connection refused") {
			errorType = "connection_refused"
		} else if strings.Contains(err.Error(), "authentication") {
			errorType = "authentication_failed"
		}
	}

	// 使用 WithLabelValues 来记录
	metrics.CacheErrors.WithLabelValues(errorType, operation).Inc()
}
