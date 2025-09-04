package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-sql-driver/mysql"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/lock"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"

	// 1. 导入内层包（apiserver/options）：聚合后的配置，供API服务器使用
	// 别名用 "apopt"（apiserver options 的缩写），避免与外层包混淆
	apopt "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	// 2. 导入外层包（pkg/options）：实际定义 BusinessLockOptions 等基础类型
	// 别名用 "pkgopt"（pkg options 的缩写）
	// 别名用 "pkgopt"（pkg options 的缩写）
	pkgopt "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
)

type UserSrv interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User) error
}

var _ UserSrv = &userService{}

type userService struct {
	store   store.Factory
	redis   *storage.RedisCluster
	options *apopt.Options
}

func newUsers(s *service) *userService {
	return &userService{
		store:   s.store,
		redis:   s.redis,
		options: s.options,
	}
}

func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	// 使用辅助函数获取上下文值

	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Create",
	)
	logger.Info("开始查询用户是否存在")
	_, err := u.store.Users().Get(ctx, user.Name, metav1.GetOptions{})
	if err == nil {
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在%s", user.Name)
	}
	logger.Info("开始执行用户创建逻辑")

	// 执行数据库操作
	err = u.store.Users().Create(ctx, user, opts)
	if err == nil {
		return nil
	}

	// 错误处理与日志记录（修复外键冲突的日志信息错误）
	switch {
	case func() bool {
		mysqlErr, ok := err.(*mysql.MySQLError)
		return ok && mysqlErr.Number == 1062
	}():
		logger.Info(
			"创建用户失败：用户已存在",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrUserAlreadyExist, "用户[%s]已经存在", user.Name)

	case errors.Is(err, gorm.ErrDuplicatedKey):
		logger.Info(
			"创建用户失败：用户已存在",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrUserAlreadyExist, "用户[%s]已经存在", user.Name)

	case errors.Is(err, gorm.ErrForeignKeyViolated):
		// 修正日志信息，与错误类型匹配
		logger.Info(
			"创建用户失败：关联数据不存在",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrUserAlreadyExist, "关联的数据不存在: %v", err)

	default:
		logger.Error(
			"创建用户失败：数据库操作异常",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrDatabase, "数据库操作失败: %v", err)
	}
}

func (u *userService) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return nil
}

func (u *userService) DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions) error {
	return nil
}
func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	// 合并所有字段，只声明一次 logger
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"operation", "Get",
	)
	logger.Info("开始执行用户查询逻辑")

	user, err := u.store.Users().Get(ctx, username, opts)
	if err != nil {
		logger.Errorw("用户查询失败:", username, "error:", err.Error())
		return nil, err
	}
	return user, nil
}
func (u *userService) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	return nil, nil
}
func (u *userService) ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	return nil, nil
}
func (u *userService) ChangePassword(ctx context.Context, user *v1.User) error {
	return nil
}

// NewUserService 创建用户服务实例
func NewUserService(store store.Factory, redis *storage.RedisCluster, opts *apopt.Options) *userService {
	return &userService{
		store:   store,
		redis:   redis,
		options: opts,
	}
}

func (u *userService) Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions) error {
	lockConfig := u.options.DistributedLock
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

	logger.Infow("开始执行用户删除")

	// 参数校验
	if username == "" {
		err := errors.WithCode(code.ErrInvalidParameter, "用户名不能为空")
		logger.Errorw("参数错误", "error", err)
		return err
	}

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
		logger.Debugw("使用全局默认锁配置",
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
		logger.Debugw("使用业务级锁配置",
			"force_lock", bizConfig.ForceLock,
			"timeout_ms", bizConfig.Timeout.Milliseconds(),
		)
	}

	// 明确判断是否需要加锁，确保needLock在所有路径被使用
	needLock := bizConfig.ForceLock || bizConfig.Enabled
	if !needLock {
		logger.Debugw("无需加锁，直接执行删除")
		return u.executeDelete(ctx, username, force, opts, logger)
	}

	// 走到这里说明需要加锁，消除未使用警告
	logger.Debugw("需要加锁，执行加锁逻辑",
		"force_lock", bizConfig.ForceLock,
		"biz_enabled", bizConfig.Enabled,
	)

	// Redis可用性检查（完整修正版）
	redisAvailable := true
	if u.redis == nil {
		redisAvailable = false
		logger.Warnw("RedisCluster实例未初始化，无法执行分布式锁操作")
	} else {
		// 1. 明确获取客户端并断言为redis.UniversalClient（适配redis v8）
		redisClient := u.redis.GetClient()

		// 2. 调用适配redis v8的pingRedis函数
		if err := pingRedis(redisClient); err != nil {
			redisAvailable = false
			logger.Errorw("Redis服务连接失败",
				"error", err,
				"client_type", fmt.Sprintf("%T", redisClient),
			)
		} else {
			logger.Debugw("Redis服务连接正常")
		}

	}
	// 处理Redis不可用降级
	if !redisAvailable {
		if !lockConfig.Fallback.Enabled {
			err := errors.WithCode(code.ErrInternal, "Redis不可用且未启用降级")
			logger.Errorw("操作失败", "error", err)
			return err
		}

		logger.Warnw("Redis不可用，触发降级", "action", lockConfig.Fallback.RedisDownAction)
		switch lockConfig.Fallback.RedisDownAction {
		case "fail":
			return errors.WithCode(code.ErrInternal, "Redis不可用，拒绝操作")
		case "skip":
			return u.executeDelete(ctx, username, force, opts, logger)
		default:
			return errors.WithCode(code.ErrInternal, "无效的降级策略")
		}
	}

	// 初始化分布式锁
	fullLockKey := fmt.Sprintf("%s%s:%s",
		lockConfig.Redis.KeyPrefix,
		bizLockKey,
		username,
	)
	redisLock := lock.NewRedisLock(u.redis, fullLockKey, bizConfig.Timeout)
	logger.Debugw("锁初始化完成",
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
		logger.Warnw("重试配置为空，使用默认值")
	}

	acquired, err := u.acquireLockWithRetry(ctx, redisLock, retryConfig, logger)
	if err != nil {
		logger.Errorw("获取锁失败", "error", err)
		return errors.WithCode(code.ErrInternal, "获取锁失败，请重试")
	}
	if !acquired {
		return errors.WithCode(code.ErrResourceConflict, "资源冲突，请稍后重试")
	}

	// 启动锁续约
	var keepAliveCancel context.CancelFunc
	if lockConfig.Redis.KeepAlive {
		keepAliveCtx, cancel := context.WithCancel(ctx)
		keepAliveCancel = cancel
		go u.keepLockAlive(keepAliveCtx, redisLock, fullLockKey, lockConfig.Redis.KeepAliveInterval, logger)
		logger.Debugw("锁续约协程启动", "interval_ms", lockConfig.Redis.KeepAliveInterval.Milliseconds())
	}

	// 确保锁释放
	defer func() {
		if keepAliveCancel != nil {
			keepAliveCancel()
		}
		if err := redisLock.Release(ctx); err != nil {
			logger.Errorw("释放锁失败", "error", err, "lock_key", fullLockKey)
		} else {
			logger.Debugw("释放锁成功", "lock_key", fullLockKey)
		}
	}()

	// 执行删除逻辑
	logger.Infow("获取锁成功，执行删除", "lock_key", fullLockKey)
	return u.executeDelete(ctx, username, force, opts, logger)
}

// executeDelete 执行删除业务逻辑
func (u *userService) executeDelete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions, logger log.Logger) error {
	var err error
	if force {
		opts = metav1.DeleteOptions{Unscoped: true}
		err = u.store.Users().DeleteForce(ctx, username, opts)
	} else {
		opts = metav1.DeleteOptions{Unscoped: false}
		err = u.store.Users().Delete(ctx, username, opts)
	}

	if err != nil {
		logger.Errorw("用户删除失败", "username", username, "error", err)
		return err
	}

	logger.Infow("用户删除成功", "username", username)
	return nil
}

// acquireLockWithRetry 带重试获取锁
func (u *userService) acquireLockWithRetry(
	ctx context.Context,
	redisLock *lock.RedisDistributedLock,
	retryConfig *pkgopt.RetryOptions,
	logger log.Logger,
) (bool, error) {
	lockKey := redisLock.GetKey()
	logger.Debugw("开始尝试获取锁",
		"lock_key", lockKey,
		"max_retry", retryConfig.MaxCount,
		"interval_ms", retryConfig.Interval.Milliseconds(),
	)

	for i := 0; i <= retryConfig.MaxCount; i++ {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("上下文取消: %w", ctx.Err())
		default:
			acquired, err := redisLock.Acquire(ctx)
			if err == nil && acquired {
				logger.Debugw("获取锁成功", "retry_count", i, "lock_key", lockKey)
				return true, nil
			}

			if err != nil && !isRetryableError(err) {
				return false, fmt.Errorf("非重试错误: %w", err)
			}

			if i < retryConfig.MaxCount {
				interval := getRetryInterval(retryConfig, i)
				logger.Debugw("获取锁失败，准备重试",
					"retry_count", i,
					"next_interval_ms", interval.Milliseconds(),
					"lock_key", lockKey,
				)
				time.Sleep(interval)
			}
		}
	}

	logger.Warnw("重试耗尽，未获取到锁", "lock_key", lockKey)
	return false, nil
}

// keepLockAlive 锁续约协程
func (u *userService) keepLockAlive(
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
			logger.Debugw("锁续约协程退出", "lock_key", lockKey)
			return
		case <-ticker.C:
			renewed, err := redisLock.Renew(ctx)
			if err != nil {
				logger.Errorw("锁续约失败", "lock_key", lockKey, "error", err)
			} else if !renewed {
				logger.Warnw("锁已失效，停止续约", "lock_key", lockKey)
				return
			} else {
				logger.Debugw("锁续约成功", "lock_key", lockKey)
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
		return errors.New("Redis客户端未初始化（nil）")
	}

	// 1. 创建带超时的上下文（redis v8要求显式传递ctx）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 2. 调用Ping方法（redis v8的UniversalClient接口明确支持该方法）
	// 注意：redis v8中Ping返回*redis.StatusCmd，需通过Err()获取错误
	statusCmd := client.Ping(ctx)
	if err := statusCmd.Err(); err != nil {
		return fmt.Errorf("Redis Ping失败: %w", err) // 包装原始错误，保留堆栈
	}

	// 3. 验证返回值（redis规范响应应为"PONG"）
	if statusCmd.Val() != "PONG" {
		return fmt.Errorf("Redis响应异常，预期'PONG'，实际: %s", statusCmd.Val())
	}

	return nil
}
