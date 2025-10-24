package user

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	authkeys "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/auth/keys"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/trace"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator/jwtvalidator"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/redis/go-redis/v9"
)

func (u *UserService) ChangePassword(ctx context.Context, user *v1.User, claims *jwtvalidator.CustomClaims, opt *options.Options) (err error) {
	serviceCtx, span := trace.StartSpan(ctx, "user-service", "change_password")
	if serviceCtx != nil {
		ctx = serviceCtx
	}
	trace.AddRequestTag(ctx, "target_user", user.Name)

	spanStatus := "success"
	businessCode := strconv.Itoa(code.ErrSuccess)
	spanDetails := map[string]any{
		"username": user.Name,
	}
	outcomeStatus := "success"
	outcomeCode := businessCode
	outcomeMessage := ""
	outcomeHTTP := http.StatusOK
	defer func() {
		if err != nil {
			spanStatus = "error"
			outcomeStatus = "error"
			if c := errors.GetCode(err); c != 0 {
				businessCode = strconv.Itoa(c)
				outcomeCode = businessCode
			} else {
				businessCode = strconv.Itoa(code.ErrUnknown)
				outcomeCode = businessCode
			}
			if msg := errors.GetMessage(err); msg != "" {
				outcomeMessage = msg
			}
			if status := errors.GetHTTPStatus(err); status != 0 {
				outcomeHTTP = status
			} else {
				outcomeHTTP = http.StatusInternalServerError
			}
		}
		if span != nil {
			trace.EndSpan(span, spanStatus, businessCode, spanDetails)
		}
		trace.RecordOutcome(ctx, outcomeCode, outcomeMessage, outcomeStatus, outcomeHTTP)
	}()

	// 判断用户是否存在 - forceRefresh=true 强制回源验证
	ruser, checkErr := u.checkUserExist(ctx, user.Name, true)
	if checkErr != nil {
		log.Warnf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查: %v", user.Name, checkErr)
	}
	if ruser != nil && (ruser.Name == RATE_LIMIT_PREVENTION || ruser.Name == BLACKLIST_SENTINEL) {
		log.Warnf("用户%s不存在,无法修改密码", user.Name)
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
		spanDetails["db_update"] = "failed"
		return err
	}
	spanDetails["db_update"] = "success"

	// 强制用户所有设备登出（带重试机制，与数据库更新保持统一风格）
	_, err = util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
		if err := u.forceLogoutAllDevices(ctx, user.ID); err != nil {
			// 如果是"没有会话"这种正常情况，不重试
			if errors.Is(err, redis.Nil) {
				log.Warnf("用户%s没有活跃会话,正常返回", user.Name)
				return nil, nil // 正常返回，不认为是错误
			}
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		spanDetails["logout_devices"] = "failed"
		log.Warnf("清理用户令牌失败: %v", err)
		// 不阻塞密码修改主流程
	} else {
		spanDetails["logout_devices"] = "success"
	}

	if blErr := u.blacklistAccessToken(ctx, claims); blErr != nil {
		spanDetails["blacklist_token"] = "failed"
		log.Warnf("写入访问令牌黑名单失败: %v", blErr)
		// 不触发回滚，记录日志即可
	} else {
		spanDetails["blacklist_token"] = "success"
	}

	log.Infof("用户%s修改密码成功，所有设备会话已强制登出", user.Name)
	return nil
}

func (u *UserService) blacklistAccessToken(ctx context.Context, claims *jwtvalidator.CustomClaims) error {
	if claims == nil {
		return nil
	}

	if claims.ID == "" || claims.UserID == "" {
		log.Warnf("访问令牌缺少jti或user_id，跳过黑名单写入")
		return nil
	}

	blacklistKey := authkeys.BlacklistKey(u.Options.JwtOptions.Blacklist_key_prefix, claims.UserID, claims.ID)
	//fullBlacklistKey := authkeys.WithGenericPrefix(blacklistKey)

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
	return nil
}

func (u *UserService) forceLogoutAllDevices(ctx context.Context, userID uint64) error {
	userIDStr := strconv.FormatUint(userID, 10)
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

	_, err := u.Redis.Eval(ctx, luaScript,
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
			log.Warnf("用户%s没有活跃会话", userIDStr)
			return redis.Nil
		}
		return errors.WithCode(code.ErrDatabase, "清理用户令牌失败: %v", err)
	}
	return nil
}
