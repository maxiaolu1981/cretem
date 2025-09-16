package user

import (
	"context"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

// Create 创建用户
func (u *Users) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {
	logger := u.createLogger(ctx, user)
	logger.Info("开始执行用户创建的数据库操作")

	startTime := time.Now()

	// 使用带重试的事务执行
	retryConfig := u.getCreateRetryConfig()

	var costMs time.Duration
	var attempt int

	err := db.Do(ctx, retryConfig, func() error {
		attempt++
		attemptStart := time.Now()
		err := u.createWithOptimizedTransaction(ctx, user, logger, attempt)
		costMs = time.Since(attemptStart)

		if err != nil && attempt < retryConfig.MaxAttempts {
			logger.Warn("创建用户尝试失败，准备重试",
				log.Int("attempt", attempt),
				log.String("error", err.Error()),
				log.Int64("cost_ms", costMs.Milliseconds()))
		}

		return err
	})

	if err != nil {
		return u.handleCreateError(err, logger, costMs)
	}

	logger.Info("用户创建成功",
		log.Int("total_attempts", attempt),
		log.Int64("total_cost_ms", time.Since(startTime).Milliseconds()),
		log.Int64("last_attempt_cost_ms", costMs.Milliseconds()))

	return nil
}

// createWithOptimizedTransaction 优化的事务创建
func (u *Users) createWithOptimizedTransaction(ctx context.Context, user *v1.User, logger log.Logger, attempt int) error {
	return u.executeInTransaction(ctx, logger, func(tx *gorm.DB) error {
		// 对于高并发场景，可以考虑先尝试快速插入
		// 如果遇到重复键错误再检查是否存在

		// 直接执行创建（依赖唯一索引来防止重复）
		if err := tx.Create(user).Error; err != nil {
			// 如果是重复键错误，明确返回用户已存在
			if u.isMySQLDuplicateError(err) {
				return errors.WithCode(code.ErrUserAlreadyExist, "用户已存在")
			}
			return err
		}

		return nil
	})
}

// getCreateRetryConfig 获取创建操作的重试配置
func (u *Users) getCreateRetryConfig() db.RetryConfig {
	return db.RetryConfig{
		MaxAttempts:       5,                      // 最大重试次数
		InitialBackoff:    100 * time.Millisecond, //初次退避时间
		MaxBackoff:        2 * time.Second,        //最大退避时间
		BackoffMultiplier: 2,                      // 减小退避倍数
		IsRetryable:       u.isCreateRetryable,    //重试判断函数
		// 添加随机抖动避免同时重试
		Jitter: 0.2,
	}
}

// isCreateRetryable 创建操作的重试判断
func (u *Users) isCreateRetryable(err error) bool {
	// 死锁错误优先处理
	if u.isMySQLDeadlockError(err) {
		return true
	}

	// 锁超时错误
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1205: // Lock wait timeout
			return true
		case 1213: // Deadlock found
			return true
		case 2002, 2003, 2006, 2013: // 连接错误
			return true
		case 2016: // 只读错误（主从同步延迟）
			return true
		}
	}

	// 超时错误
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// 重复键错误不重试
	if u.isMySQLDuplicateError(err) {
		return false
	}

	// 其他错误使用默认判断
	return db.DefaultIsRetryable(err)
}

// createLogger 创建专用的日志实例
func (u *Users) createLogger(ctx context.Context, user *v1.User) log.Logger {
	return log.L(ctx).WithValues(
		"layer", "store",
		"component", "users",
		"table", "user",
		"operation", "create",
		"target_username", user.Name,
		"user_status", user.Status,
	)
}

// getLogger 创建查询专用的日志实例
func (u *Users) getLogger(ctx context.Context) log.Logger {
	return log.L(ctx).WithValues(
		"layer", "store",
		"component", "users",
		"table", "user",
		"operation", "get",
	)
}

// createTimeoutContext 创建超时上下文
func (u *Users) createTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	defaultTimeout := 3 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < defaultTimeout {
			return context.WithTimeout(ctx, remaining)
		}
	}

	return context.WithTimeout(ctx, defaultTimeout)
}

// executeSingleGet 执行单次查询
func (u *Users) executeSingleGet(ctx context.Context, username string) (*v1.User, error) {
	user := &v1.User{}
	err := u.db.WithContext(ctx).
		Where("name = ? and status = 1", username).
		First(user).Error

	if err != nil {
		return nil, err
	}

	return user, nil
}

// isRetryableError 自定义重试错误判断（可以覆盖默认行为）
func (u *Users) isRetryableError(err error) bool {
	// 首先使用默认判断
	if db.DefaultIsRetryable(err) {
		return true
	}

	return false
}

// 增强的错误处理
func (u *Users) handleCreateError(err error, logger log.Logger, cost time.Duration) error {
	logger = logger.WithValues("cost_ms", cost.Milliseconds())

	var mysqlErr *mysql.MySQLError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		logger.Warn("用户创建操作超时")
		return errors.WithCode(code.ErrDatabaseTimeout, "创建用户超时")

	case errors.As(err, &mysqlErr) && mysqlErr.Number == 1213:
		logger.Warn("用户创建遇到死锁")
		return errors.WithCode(code.ErrDatabaseDeadlock, "系统繁忙，请稍后重试")

	case u.isMySQLDuplicateError(err):
		logger.Info("创建用户失败：用户已存在")
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已存在")

	default:
		logger.Error("用户创建操作失败", log.String("error", err.Error()))
		return errors.WithCode(code.ErrDatabase, "数据库操作失败")
	}
}

// handleGetError 处理查询错误
func (u *Users) handleGetError(err error, username string, logger log.Logger, cost time.Duration) error {
	logger = logger.WithValues("cost_ms", cost.Milliseconds())

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		logger.Info("用户不存在",
			log.String("username", username),
			log.String("error", err.Error()),
		)

		return errors.WithCode(code.ErrUserNotFound, "用户[%s]不存在", username)

	case errors.Is(err, context.DeadlineExceeded):
		logger.Warn("用户查询操作超时",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrDatabaseTimeout, "查询用户超时")

	default:
		logger.Error("用户查询操作失败",
			log.String("error", err.Error()),
		)
		return errors.WithCode(code.ErrDatabase, "数据库查询失败: %v", err)
	}
}

// logCreateSuccess 记录创建成功日志
func (u *Users) logCreateSuccess(logger log.Logger, cost time.Duration) {
	logger.Info("用户创建成功",
		//	log.String("username", user.Name),
		log.Int64("cost_ms", cost.Milliseconds()),
	)
}

// logGetSuccess 记录查询成功日志
func (u *Users) logGetSuccess(user *v1.User, logger log.Logger, cost time.Duration) {
	logger.Debugf("用户查询成功%v%v%v",
		log.Int64("cost_ms", cost.Milliseconds()),
		log.Uint64("user_id", user.ID),
		log.Any("user_status", user.Status),
	)
}

func (u *Users) executeInTransaction(ctx context.Context, logger log.Logger, fn func(tx *gorm.DB) error) error {
	txStartTime := time.Now()
	logger.Debugw("开始数据库事务", "tx_start_time", txStartTime)

	tx := u.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		txDuration := time.Since(txStartTime)
		logger.Errorw("开始事务失败", "error", tx.Error, "tx_duration_ms", txDuration.Milliseconds())
		return errors.WithCode(code.ErrDatabase, "开始事务失败")
	}

	var completed bool
	completedPtr := &completed

	defer func() {
		if !*completedPtr {
			if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
				logger.Errorw("回滚事务失败", "error", rollbackErr)
			} else {
				logger.Warnw("事务已回滚")
			}
		}
	}()

	// 执行事务操作
	fnStart := time.Now()
	if err := fn(tx); err != nil {
		fnDuration := time.Since(fnStart)
		logger.Debugw("事务操作执行失败", "error", err, "fn_duration_ms", fnDuration.Milliseconds())
		// ✅ 修复：设置completed为false，让defer回滚
		*completedPtr = false // 确保defer中的回滚会执行
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		logger.Errorw("提交事务失败", "error", err)
		completed = false // 确保回滚
		return errors.WithCode(code.ErrDatabase, "提交事务失败")
	}

	*completedPtr = true // 提交成功，不需要回滚
	logger.Debugw("事务提交成功")
	return nil
}
