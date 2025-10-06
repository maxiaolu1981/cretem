package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	apiserveropts "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
)

// 查询当前限流配置
func RegisterRateLimitAdminHandlers(rg *gin.RouterGroup, redisCluster *storage.RedisCluster, opts *apiserveropts.Options) {
	rg.GET("/ratelimit/write", func(c *gin.Context) {
		if !isLocalOrDebug(c, opts) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		key := ""
		metaKey := ""
		if redisCluster != nil {
			key = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit")
			metaKey = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit:meta")
		} else {
			key = "ratelimit:write:global_limit"
			metaKey = "ratelimit:write:global_limit:meta"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		if redisCluster == nil || redisCluster.GetClient() == nil {
			core.WriteResponse(c, nil, gin.H{"value": nil, "source": "no_redis"})
			return
		}
		val, err := redisCluster.GetClient().Get(ctx, key).Result()
		if err != nil {
			// if not found, return nil value with no error
			core.WriteResponse(c, nil, gin.H{"value": nil, "error": err.Error()})
			return
		}
		ttl, _ := redisCluster.GetClient().TTL(ctx, key).Result()
		source := "unknown"
		if metaVal, err := redisCluster.GetClient().Get(ctx, metaKey).Result(); err == nil {
			source = metaVal
		}
		core.WriteResponse(c, nil, gin.H{"value": val, "ttl_seconds": int(ttl.Seconds()), "source": source})
	})

	type setReq struct {
		Value int `json:"value" binding:"required,min=1"`
		TTL   int `json:"ttl_seconds" binding:"omitempty,min=1"`
	}
	//删除限流配置
	rg.POST("/ratelimit/write", func(c *gin.Context) {
		// If AdminToken is set, require it. Otherwise fall back to local/debug only.
		if opts.ServerRunOptions.AdminToken != "" {
			provided := c.GetHeader("X-Admin-Token")
			if provided == "" || provided != opts.ServerRunOptions.AdminToken {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		} else {
			if !isLocalOrDebug(c, opts) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		var req setReq
		if err := c.ShouldBindJSON(&req); err != nil {
			core.WriteResponse(c, err, nil)
			return
		}
		key := ""
		metaKey := ""
		if redisCluster != nil {
			key = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit")
			metaKey = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit:meta")
		} else {
			key = "ratelimit:write:global_limit"
			metaKey = "ratelimit:write:global_limit:meta"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if redisCluster == nil || redisCluster.GetClient() == nil {
			core.WriteResponse(c, nil, gin.H{"result": "no_redis"})
			return
		}
		if req.TTL > 0 {
			if err := redisCluster.GetClient().Set(ctx, key, req.Value, time.Duration(req.TTL)*time.Second).Err(); err != nil {
				core.WriteResponse(c, err, nil)
				return
			}
			// store source info with same TTL
			source := requesterSource(c, opts)
			_ = redisCluster.GetClient().Set(ctx, metaKey, source, time.Duration(req.TTL)*time.Second).Err()
		} else {
			if err := redisCluster.GetClient().Set(ctx, key, req.Value, 0).Err(); err != nil {
				core.WriteResponse(c, err, nil)
				return
			}
			source := requesterSource(c, opts)
			_ = redisCluster.GetClient().Set(ctx, metaKey, source, 0).Err()
		}
		log.Debugf("Set global write ratelimit=%d ttl=%d", req.Value, req.TTL)
		core.WriteResponse(c, nil, gin.H{"result": "ok"})
	})

	// DELETE handler to remove the global limit key
	rg.DELETE("/ratelimit/write", func(c *gin.Context) {
		// auth same as POST
		if opts.ServerRunOptions.AdminToken != "" {
			provided := c.GetHeader("X-Admin-Token")
			if provided == "" || provided != opts.ServerRunOptions.AdminToken {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		} else {
			if !isLocalOrDebug(c, opts) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		key := ""
		metaKey := ""
		if redisCluster != nil {
			key = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit")
			metaKey = fmt.Sprintf("%s%s", redisCluster.KeyPrefix, "ratelimit:write:global_limit:meta")
		} else {
			key = "ratelimit:write:global_limit"
			metaKey = "ratelimit:write:global_limit:meta"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if redisCluster == nil || redisCluster.GetClient() == nil {
			core.WriteResponse(c, nil, gin.H{"result": "no_redis"})
			return
		}
		// delete both key and meta
		if err := redisCluster.GetClient().Del(ctx, key).Err(); err != nil {
			core.WriteResponse(c, err, nil)
			return
		}
		_ = redisCluster.GetClient().Del(ctx, metaKey).Err()
		core.WriteResponse(c, nil, gin.H{"result": "deleted"})
	})
}

func isLocalOrDebug(c *gin.Context, opts *apiserveropts.Options) bool {
	if opts.ServerRunOptions.Mode == "debug" {
		return true
	}
	ip := c.ClientIP()
	return ip == "127.0.0.1" || ip == "::1" || ip == "localhost"
}

// requesterSource returns a short string describing who set the value: prefer token if present, else client IP
func requesterSource(c *gin.Context, opts *apiserveropts.Options) string {
	if opts.ServerRunOptions.AdminToken != "" {
		provided := c.GetHeader("X-Admin-Token")
		if provided != "" && provided == opts.ServerRunOptions.AdminToken {
			return "token"
		}
	}
	return c.ClientIP()
}
