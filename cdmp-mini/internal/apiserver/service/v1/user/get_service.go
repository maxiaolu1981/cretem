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

	//检查bloom过滤器
	bloom, err := bloomfilter.GetFilter()
	if err != nil {
		metrics.RecordBloomFilterCheck("username", "error", 0)
		metrics.SetBloomFilterStatus("username", false)
	}
	if bloom != nil {
		start := time.Now()
		//如果没有找到，，直接返回
		if !bloom.Test("username", username) {
			// 第一次查询不存在的用户会进入这里
			duration := time.Since(start)
			metrics.RecordBloomFilterCheck("username", "miss", duration)
			//记录布隆过滤器防护次数
			metrics.BloomFilterPreventions.WithLabelValues("username").Inc()
			// 设置Redis空值缓存
			go u.cacheNullValue(cacheKey)
			return nil, nil
		}
	}

	// 2. 使用singleflight包装整个查询
	result, err, shared := u.group.Do(cacheKey, func() (interface{}, error) {
		return u.getUserWithCache(ctx, username, cacheKey, opts, opt)
	})

	if shared {
		log.Infof("请求被合并，共享结果: %s", username)
	}

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
		log.Warnw("缓存查询失败，降级到数据库查询", "error", err.Error())
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
	log.Infof("缓存未命中，查询数据库")
	metrics.CacheHits.WithLabelValues("no_record").Inc()
	return u.getUserFromDBAndSetCache(ctx, username, cacheKey, opts, opt)
}
