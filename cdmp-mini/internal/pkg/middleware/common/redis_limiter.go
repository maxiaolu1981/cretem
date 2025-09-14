package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
)

// 本地计数器减少Redis压力
var localLoginCounter int64

// buildRedisKey 构建完整的Redis键（处理前缀）
func buildRedisKey(redisCluster *storage.RedisCluster, key string) string {
	if redisCluster.KeyPrefix != "" {
		return redisCluster.KeyPrefix + key
	}
	return key
}

// LoginRateLimiter 优化的登录限流中间件
func LoginRateLimiter(redisCluster *storage.RedisCluster, limit int, window time.Duration) gin.HandlerFunc {
	windowSec := int64(window.Seconds())

	// 优化的Lua脚本 - 使用计数器算法，性能更好
	luaScript := `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local current_time = tonumber(ARGV[3])

-- 使用计数器算法
local count = redis.call('GET', key)
if count == false then
    -- Key不存在，设置新计数器和过期时间
    redis.call('SETEX', key, window, 1)
    return 0
else
    count = tonumber(count)
    if count < limit then
        -- 计数增加
        redis.call('INCR', key)
        return 0
    else
        -- 超过限制
        return 1
    end
end
`

	return func(c *gin.Context) {
		// 第一层：本地快速检查（减少Redis压力）
		currentCount := atomic.AddInt64(&localLoginCounter, 1)
		defer atomic.AddInt64(&localLoginCounter, -1)

		// 本地粗略限流，避免过多请求打到Redis
		if currentCount > int64(limit*3) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded (local protection)",
			})
			return
		}

		identifier := "ip:" + c.ClientIP()
		rateLimitKey := buildRedisKey(redisCluster, "ratelimit:login:"+identifier)

		// 设置更短的超时时间（100ms）
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// 执行Lua脚本
		result := redisCluster.GetClient().Eval(ctx, luaScript,
			[]string{rateLimitKey},
			limit, windowSec, time.Now().Unix(),
		)

		val, err := result.Int()
		if err != nil {
			// Redis操作失败时的降级策略
			if errors.Is(err, context.DeadlineExceeded) {
				log.Warnf("Redis限流操作超时，降级处理: %v", err)
				// 超时时根据本地计数决定是否限流
				if currentCount > int64(limit) {
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
						"error": "Rate limit exceeded (degraded mode)",
					})
					return
				}
				// 超时但本地计数未超限，放行
				c.Next()
				return
			}

			// 其他Redis错误
			log.Errorf("Redis限流错误: %v", err)
			// 错误时放行，避免影响正常业务（安全考虑）
			c.Next()
			return
		}

		// 根据Lua脚本返回值判断是否限流
		if val == 1 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"limit":       limit,
				"window":      window.String(),
				"retry_after": fmt.Sprintf("%.0f seconds", window.Seconds()),
			})
			return
		}

		// 允许通过
		c.Next()
	}
}

// LoginRateLimiterByUser 按用户ID限流的优化版本
func LoginRateLimiterByUser(redisCluster *storage.RedisCluster, limit int, window time.Duration) gin.HandlerFunc {
	windowSec := int64(window.Seconds())

	luaScript := `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

local count = redis.call('GET', key)
if count == false then
    redis.call('SETEX', key, window, 1)
    return 0
else
    count = tonumber(count)
    if count < limit then
        redis.call('INCR', key)
        return 0
    else
        return 1
    end
end
`

	return func(c *gin.Context) {
		// 从上下文中获取用户ID
		userID, exists := c.Get("userID")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		identifier := fmt.Sprintf("user:%v", userID)
		rateLimitKey := buildRedisKey(redisCluster, "ratelimit:login:"+identifier)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		result := redisCluster.GetClient().Eval(ctx, luaScript,
			[]string{rateLimitKey},
			limit, windowSec,
		)

		val, err := result.Int()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Warnf("用户限流Redis超时: %v", err)
				// 超时放行，避免影响用户体验
				c.Next()
				return
			}
			log.Errorf("用户限流Redis错误: %v", err)
			c.Next()
			return
		}

		if val == 1 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "User rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

// CleanupRateLimit 清理限流数据（用于测试后清理）
func CleanupRateLimit(redisCluster *storage.RedisCluster) {
	ctx := context.Background()
	// 清理所有限流相关的key
	pattern := buildRedisKey(redisCluster, "ratelimit:login:*")
	keys, err := redisCluster.GetClient().Keys(ctx, pattern).Result()
	if err == nil && len(keys) > 0 {
		redisCluster.GetClient().Del(ctx, keys...)
	}
	log.Infof("清理了 %d 个限流key", len(keys))
}
