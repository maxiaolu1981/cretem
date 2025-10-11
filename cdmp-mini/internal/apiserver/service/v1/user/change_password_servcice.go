package user

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	authkeys "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/auth/keys"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator/jwtvalidator"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) ChangePassword(ctx context.Context, user *v1.User, claims *jwtvalidator.CustomClaims, opt *options.Options) error {
	// 判断用户是否存在 - forceRefresh=true 强制回源验证
	ruser, err := u.checkUserExist(ctx, user.Name, true)
	if err != nil {
		log.Debugf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查: %v", user.Name, err)
	}
	if ruser != nil && ruser.Name == RATE_LIMIT_PREVENTION {
		log.Debugf("用户%s不存在,无法修改密码", user.Name)
		return errors.WithCode(code.ErrUserNotFound, "用户不存在")
	}

	// 更新数据库
	_, err = util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
		if err := u.Store.Users().Update(ctx, user, metav1.UpdateOptions{}, opt); err != nil {
			return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	// 强制用户所有设备登出（带重试机制，与数据库更新保持统一风格）
	_, err = util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
		if err := u.forceLogoutAllDevices(ctx, user.ID); err != nil {
			// 如果是"没有会话"这种正常情况，不重试
			if errors.Is(err, redis.Nil) {
				log.Debugf("用户%s没有活跃会话", user.Name)
				return nil, nil // 正常返回，不认为是错误
			}
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		log.Warnf("清理用户令牌失败: %v", err)
		// 不阻塞密码修改主流程
	}

	if err := u.blacklistAccessToken(ctx, claims); err != nil {
		log.Warnf("写入访问令牌黑名单失败: %v", err)
		// 不触发回滚，记录日志即可
	}

	log.Infof("用户%s修改密码成功，所有设备会话已强制登出", user.Name)
	return nil
}

func (u *UserService) blacklistAccessToken(ctx context.Context, claims *jwtvalidator.CustomClaims) error {
	if claims == nil {
		return nil
	}

	if claims.ID == "" || claims.UserID == "" {
		log.Debug("访问令牌缺少jti或user_id，跳过黑名单写入")
		return nil
	}

	blacklistKey := authkeys.BlacklistKey(u.Options.JwtOptions.Blacklist_key_prefix, claims.UserID, claims.ID)
	fullBlacklistKey := authkeys.WithGenericPrefix(blacklistKey)

	var ttl time.Duration
	if claims.ExpiresAt != nil {
		ttl = time.Until(claims.ExpiresAt.Time) + time.Hour
	}
	if ttl <= 0 {
		ttl = u.Options.JwtOptions.Timeout + time.Hour
	}
	if ttl <= 0 {
		ttl = time.Hour
	}

	if err := u.Redis.SetKey(ctx, blacklistKey, "1", ttl); err != nil {
		return errors.WithCode(code.ErrDatabase, "写入访问令牌黑名单失败: %v", err)
	}

	log.Debugf("访问令牌已加入黑名单: key=%s, ttl=%s", fullBlacklistKey, ttl.String())
	return nil
}

func (u *UserService) forceLogoutAllDevices(ctx context.Context, userID uint64) error {
	userIDStr := strconv.FormatUint(userID, 10)

	log.Debugf("开始清理用户%s的所有设备会话", userIDStr)

	userSessionsKey := authkeys.UserSessionsKey(userIDStr)
	refreshPrefix := authkeys.WithGenericPrefix(authkeys.RefreshTokenPrefix(userIDStr))

	luaScript := `
		-- KEYS[1]: 用户会话集合完整键名
		-- ARGV[1]: Refresh Token 键前缀（含哈希标签，以冒号结尾）

		local userSessionsKey = KEYS[1]
		local refreshTokenPrefix = ARGV[1]

		redis.log(redis.LOG_NOTICE, "开始清理用户会话: " .. userSessionsKey)

		local tokens = redis.call('SMEMBERS', userSessionsKey)
		redis.log(redis.LOG_NOTICE, "找到 " .. #tokens .. " 个refresh token")

		for _, token in ipairs(tokens) do
			local rtKey = refreshTokenPrefix .. token
			redis.call('DEL', rtKey)
			redis.log(redis.LOG_NOTICE, "已删除refresh token: " .. rtKey)
		end

		local delResult = redis.call('DEL', userSessionsKey)
		redis.log(redis.LOG_NOTICE, "删除用户会话集合结果: " .. delResult)

		return #tokens
	`

	log.Debugf("执行Lua脚本，参数: userID=%s", userIDStr)

	result, err := u.Redis.Eval(ctx, luaScript,
		[]string{
			userSessionsKey,
		},
		[]interface{}{
			refreshPrefix,
		},
	)

	if err != nil {
		log.Errorf("Lua脚本执行失败: %v", err)
		if errors.Is(err, redis.Nil) {
			log.Debugf("用户%s没有活跃会话", userIDStr)
			return redis.Nil
		}
		return errors.WithCode(code.ErrDatabase, "清理用户令牌失败: %v", err)
	}

	tokenCount := 0
	if result != nil {
		tokenCount = int(result.(int64))
	}

	log.Debugf("用户%s密码修改，清理了%d个refresh token", userIDStr, tokenCount)
	return nil
}
