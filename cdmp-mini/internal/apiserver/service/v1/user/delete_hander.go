package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/lock"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	pkgopt "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
)

func (u *UserService) DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions) error {
	return nil
}

func (u *UserService) Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions) error {
	lockConfig := u.Options.DistributedLock
	bizLockKey := "user:delete"

	// 初始化日志（适配log包接口）
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Delete",
		"username", username,
		"biz_lock_key", bizLockKey,
	)

	// 监控锁生命周期
	lockStartTime := time.Now()
	defer func() {
		lockCost := time.Since(lockStartTime)
		logger.Infow("锁操作完成",
			"lock_cost_ms", lockCost.Milliseconds(),
			"slow_threshold_ms", lockConfig.Monitoring.SlowThreshold.Milliseconds(),
		)

		if lockConfig.Monitoring.Enabled && lockCost > lockConfig.Monitoring.SlowThreshold {
			log.Warnw("分布式锁操作超时",
				"biz_lock_key", bizLockKey,
				"cost_ms", lockCost.Milliseconds(),
				"threshold_ms", lockConfig.Monitoring.SlowThreshold.Milliseconds(),
			)
		}
	}()

	// 解析业务锁配置
	var bizConfig *pkgopt.BusinessLockOptions
	bizConfig, exists := lockConfig.Business[bizLockKey]

	if !exists {
		bizConfig = &pkgopt.BusinessLockOptions{
			Enabled:   lockConfig.Enabled,
			Timeout:   lockConfig.DefaultTimeout,
			Retry:     lockConfig.DefaultRetry,
			ForceLock: false,
		}
		logger.Debugw("服务层:使用全局默认锁配置",
			"timeout_ms", bizConfig.Timeout.Milliseconds(),
		)
	} else {
		// 补全业务配置
		if bizConfig.Timeout <= 0 {
			bizConfig.Timeout = lockConfig.DefaultTimeout
		}
		if bizConfig.Retry == nil {
			bizConfig.Retry = lockConfig.DefaultRetry
		} else {
			if bizConfig.Retry.MaxCount <= 0 {
				bizConfig.Retry.MaxCount = lockConfig.DefaultRetry.MaxCount
			}
			if bizConfig.Retry.Interval <= 0 {
				bizConfig.Retry.Interval = lockConfig.DefaultRetry.Interval
			}
		}
		if !bizConfig.ForceLock && !bizConfig.Enabled {
			bizConfig.Enabled = lockConfig.Enabled
		}
		logger.Debugw("服务层:使用业务级锁配置",
			"force_lock", bizConfig.ForceLock,
			"timeout_ms", bizConfig.Timeout.Milliseconds(),
		)
	}

	// 明确判断是否需要加锁，确保needLock在所有路径被使用
	needLock := bizConfig.ForceLock || bizConfig.Enabled
	if !needLock {
		logger.Debugf("服务层:开始删除用户[%s](未加锁)", username)
		return u.executeDelete(ctx, username, force, opts)
	}

	// 走到这里说明需要加锁，消除未使用警告
	logger.Debugw("服务层:需要加锁，执行加锁逻辑",
		"force_lock", bizConfig.ForceLock,
		"biz_enabled", bizConfig.Enabled,
	)

	// Redis可用性检查（完整修正版）
	redisAvailable := true
	if u.Redis == nil {
		redisAvailable = false
		logger.Warnw("服务层:RedisCluster实例未初始化，无法执行分布式锁操作")
	} else {
		// 1. 明确获取客户端并断言为redis.UniversalClient（适配redis v8）
		redisClient := u.Redis.GetClient()

		// 2. 调用适配redis v8的pingRedis函数
		if err := pingRedis(redisClient); err != nil {
			redisAvailable = false
			logger.Errorw("服务层:Redis服务连接失败",
				"error", err,
				"client_type", fmt.Sprintf("%T", redisClient),
			)
		} else {
			logger.Debugw("服务层:Redis服务连接正常")
		}

	}
	// 处理Redis不可用降级
	if !redisAvailable {
		if !lockConfig.Fallback.Enabled {
			err := errors.WithCode(code.ErrInternal, "服务层:Redis不可用且未启用降级")
			logger.Errorw("服务层:操作失败", "error", err)
			return err
		}

		logger.Warnw("服务层:Redis不可用，触发降级", "action", lockConfig.Fallback.RedisDownAction)
		switch lockConfig.Fallback.RedisDownAction {
		case "fail":
			return errors.WithCode(code.ErrInternal, "服务层:Redis不可用，拒绝操作")
		case "skip":
			return u.executeDelete(ctx, username, force, opts)
		default:
			return errors.WithCode(code.ErrInternal, "服务层:无效的降级策略")
		}
	}

	// 初始化分布式锁
	fullLockKey := fmt.Sprintf("%s%s:%s",
		lockConfig.Redis.KeyPrefix,
		bizLockKey,
		username,
	)
	redisLock := lock.NewRedisLock(u.Redis, fullLockKey, bizConfig.Timeout)
	logger.Debugw("服务层:锁初始化完成",
		"full_lock_key", fullLockKey,
		"timeout_ms", bizConfig.Timeout.Milliseconds(),
	)

	// 获取锁（带重试）
	retryConfig := bizConfig.Retry
	if retryConfig == nil {
		retryConfig = &pkgopt.RetryOptions{
			MaxCount:    3,
			Interval:    100 * time.Millisecond,
			BackoffType: "fixed",
		}
		logger.Warnw("服务层:重试配置为空，使用默认值")
	}

	acquired, err := u.acquireLockWithRetry(ctx, redisLock, retryConfig, logger)
	if err != nil {
		logger.Errorw("服务层:获取锁失败", "error", err)
		return errors.WithCode(code.ErrInternal, "服务层:获取锁失败，请重试")
	}
	if !acquired {
		return errors.WithCode(code.ErrResourceConflict, "服务层:资源冲突，请稍后重试")
	}

	// 启动锁续约
	var keepAliveCancel context.CancelFunc
	if lockConfig.Redis.KeepAlive {
		keepAliveCtx, cancel := context.WithCancel(ctx)
		keepAliveCancel = cancel
		go u.keepLockAlive(keepAliveCtx, redisLock, fullLockKey, lockConfig.Redis.KeepAliveInterval, logger)
		logger.Debugw("服务层:锁续约协程启动", "interval_ms", lockConfig.Redis.KeepAliveInterval.Milliseconds())
	}

	// 确保锁释放
	defer func() {
		if keepAliveCancel != nil {
			keepAliveCancel()
		}
		if err := redisLock.Release(ctx); err != nil {
			logger.Errorw("服务层:释放锁失败", "error", err, "lock_key", fullLockKey)
		} else {
			logger.Debugw("服务层:释放锁成功", "lock_key", fullLockKey)
		}
	}()

	// 执行删除逻辑
	logger.Debugw("服务层:获取锁成功，执行删除", "lock_key", fullLockKey)
	return u.executeDelete(ctx, username, force, opts)
}

// executeDelete 执行删除业务逻辑
func (u *UserService) executeDelete(ctx context.Context, username string,
	force bool, opts metav1.DeleteOptions) error {
	var err error
	if force {
		opts = metav1.DeleteOptions{Unscoped: true}
		err = u.Store.Users().DeleteForce(ctx, username, opts)
	} else {
		opts = metav1.DeleteOptions{Unscoped: false}
		err = u.Store.Users().Delete(ctx, username, opts)
	}

	if err != nil {
		//	logger.Errorw("用户删除失败", "username", username, "error", err)
		return err
	}

	//	logger.Infow("用户删除成功", "username", username)
	return nil
}

// acquireLockWithRetry 带重试获取锁
func (u *UserService) acquireLockWithRetry(
	ctx context.Context,
	redisLock *lock.RedisDistributedLock,
	retryConfig *pkgopt.RetryOptions,
	logger log.Logger,
) (bool, error) {
	lockKey := redisLock.GetKey()
	logger.Debugw("服务层:开始尝试获取锁",
		"lock_key", lockKey,
		"max_retry", retryConfig.MaxCount,
		"interval_ms", retryConfig.Interval.Milliseconds(),
	)

	for i := 0; i <= retryConfig.MaxCount; i++ {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("服务层:上下文取消: %w", ctx.Err())
		default:
			acquired, err := redisLock.Acquire(ctx)
			if err == nil && acquired {
				logger.Debugw("服务层:获取锁成功", "retry_count", i, "lock_key", lockKey)
				return true, nil
			}

			if err != nil && !isRetryableError(err) {
				return false, fmt.Errorf("服务层:非重试错误: %w", err)
			}

			if i < retryConfig.MaxCount {
				interval := getRetryInterval(retryConfig, i)
				logger.Debugw("服务层:获取锁失败，准备重试",
					"retry_count", i,
					"next_interval_ms", interval.Milliseconds(),
					"lock_key", lockKey,
				)
				time.Sleep(interval)
			}
		}
	}

	logger.Warnw("服务层:重试耗尽，未获取到锁", "lock_key", lockKey)
	return false, nil
}

// keepLockAlive 锁续约协程
func (u *UserService) keepLockAlive(
	ctx context.Context,
	redisLock *lock.RedisDistributedLock,
	lockKey string,
	interval time.Duration,
	logger log.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debugw("服务层:锁续约协程退出", "lock_key", lockKey)
			return
		case <-ticker.C:
			renewed, err := redisLock.Renew(ctx)
			if err != nil {
				logger.Errorw("服务层:锁续约失败", "lock_key", lockKey, "error", err)
			} else if !renewed {
				logger.Warnw("服务层:锁已失效，停止续约", "lock_key", lockKey)
				return
			} else {
				logger.Debugw("服务层:锁续约成功", "lock_key", lockKey)
			}
		}
	}
}

// isRetryableError 判断是否可重试错误
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	retryable := []string{
		"redis: nil",
		"i/o timeout",
		"connection refused",
		"context deadline exceeded",
	}
	for _, s := range retryable {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// getRetryInterval 计算重试间隔
func getRetryInterval(retryConfig *pkgopt.RetryOptions, retryCount int) time.Duration {
	switch retryConfig.BackoffType {
	case "exponential":
		return time.Duration(1<<retryCount) * retryConfig.Interval
	default:
		return retryConfig.Interval
	}
}

// 关键修正：适配redis v8的pingRedis函数
func pingRedis(client redis.UniversalClient) error {
	if client == nil {
		return errors.New("服务层:Redis客户端未初始化（nil）")
	}

	// 1. 创建带超时的上下文（redis v8要求显式传递ctx）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 2. 调用Ping方法（redis v8的UniversalClient接口明确支持该方法）
	// 注意：redis v8中Ping返回*redis.StatusCmd，需通过Err()获取错误
	statusCmd := client.Ping(ctx)
	if err := statusCmd.Err(); err != nil {
		return fmt.Errorf("服务层:Redis Ping失败: %w", err) // 包装原始错误，保留堆栈
	}

	// 3. 验证返回值（redis规范响应应为"PONG"）
	if statusCmd.Val() != "PONG" {
		return fmt.Errorf("服务层:Redis响应异常，预期'PONG'，实际: %s", statusCmd.Val())
	}

	return nil
}
