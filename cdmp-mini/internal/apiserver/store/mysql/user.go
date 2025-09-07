package mysql

import (
	"context"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/db"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

// 这个 users 结构体的设计与之前提到的 datastore 类似，但更聚焦于 “用户” 这一特定资源的数据库操作，通过持有 *gorm.DB 实例，专门封装与用户相关的数据库交互逻辑。
// 与 datastore 的区别：datastore 通常是全局或通用的数据库连接管理器，而 users 是更细分的 “用户资源操作类”，直接依赖 *gorm.DB 而非 datastore，结构更简洁。
// 仓库工人

// 作用：通过用户名（username）和查询选项（opts）从数据库中查询状态有效的用户，并返回符合条件的用户信息。
// 核心依赖：u.db（*gorm.DB）执行数据库查询，ctx（context.Context）用于传递上下文（如超时控制、追踪信息）。

type users struct {
	db *gorm.DB
}

func newUsers(ds *datastore) *users {
	return &users{
		db: ds.DB,
	}
}

func (u *users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return nil
}

func (u *users) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	return nil
}

func (u *users) DeleteForce(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	logger := log.L(ctx).WithValues(
		"method", "DeleteForce",
		"unscoped", true,
	)

	startTime := time.Now()
	logger.Infow("存储层:开始用户删除操作", "start_time", startTime)

	err := u.executeInTransaction(ctx, logger, func(tx *gorm.DB) error {
		// 1. 删除关联数据
		assocStart := time.Now()
		if err := u.deleteUserAssociations(tx, username, opts); err != nil {
			return err
		}
		logger.Infow("存储层:关联数据删除完成", "duration_ms", time.Since(assocStart).Milliseconds())

		// 2. 删除用户主体数据
		mainStart := time.Now()
		if err := u.deleteUserMainData(tx, username, opts); err != nil {
			return err
		}
		logger.Infow("存储层:主体数据删除完成", "duration_ms", time.Since(mainStart).Milliseconds())

		return nil
	})

	totalDuration := time.Since(startTime)
	if err != nil {
		logger.Errorw("存储层:用户删除失败", "error", err, "total_duration_ms", totalDuration.Milliseconds(), "status", "failed")
		return err
	}

	//logger.Infow("用户删除成功", "total_duration_ms", totalDuration.Milliseconds(), "status", "success")
	return nil
}

// deleteUserAssociations 删除用户关联数据（修复tx参数使用）
func (u *users) deleteUserAssociations(tx *gorm.DB, username string, opts metav1.DeleteOptions) error {
	logger := log.L(context.Background()).WithValues("operation", "delete_user_associations", "username", username)

	// 使用事务tx
	pol := newPolices(&datastore{DB: tx}) // 根据实际情况使用 db 或 DB

	policyStart := time.Now()
	if err := pol.DeleteByUser(context.Background(), username, opts); err != nil {
		policyDuration := time.Since(policyStart)
		logger.Errorw("存储层:删除用户策略失败", "error", err, "policy_duration_ms", policyDuration.Milliseconds())
		return errors.Wrap(err, "存储层:删除用户策略失败")
	}

	policyDuration := time.Since(policyStart)
	logger.Debugw("存储层:用户策略删除完成", "policy_duration_ms", policyDuration.Milliseconds())

	return nil
}

// deleteUserMainData 删除用户主体数据（带计时）
func (u *users) deleteUserMainData(tx *gorm.DB, username string, opts metav1.DeleteOptions) error {
	logger := log.L(context.Background()).WithValues(
		"operation", "delete_user_main_data",
		"username", username,
		"unscoped", opts.Unscoped,
	)

	db := tx
	if opts.Unscoped {
		db = db.Unscoped()
		logger.Debug("存储层:使用硬删除模式")
	}

	// 记录删除操作时间
	deleteStart := time.Now()
	result := db.Where("name = ?", username).Delete(&v1.User{})
	deleteDuration := time.Since(deleteStart)

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logger.Errorw("存储层:删除用户失败",
			"error", result.Error,
			"delete_duration_ms", deleteDuration.Milliseconds(),
		)
		return errors.WithCode(code.ErrDatabase, result.Error.Error())
	}

	if result.RowsAffected == 0 {
		logger.Warnw("存储层:未找到要删除的用户",
			"delete_duration_ms", deleteDuration.Milliseconds(),
		)
		return errors.WithCode(code.ErrUserNotFound, "存储层:用户不存在")
	}

	logger.Infow("存储层:用户主体数据删除成功",
		"rows_affected", result.RowsAffected,
		"delete_duration_ms", deleteDuration.Milliseconds(),
	)

	return nil
}

func (u *users) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error {
	return nil
}

func (u *users) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	return nil, nil
}

// Create 创建用户
func (u *users) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	logger := u.createLogger(ctx, user)
	logger.Info("开始执行用户创建的数据库操作")

	// 设置超时上下文
	dbCtx, cancel := u.createTimeoutContext(ctx)
	defer cancel()

	startTime := time.Now()

	// 使用retry工具执行数据库操作
	err := db.Do(dbCtx, db.DefaultRetryConfig, func() error {
		return u.db.WithContext(dbCtx).Create(user).Error
	})

	costMs := time.Since(startTime)

	if err != nil {
		return u.handleCreateError(err, logger, costMs)
	}

	u.logCreateSuccess(logger, costMs)
	return nil
}

// Get 查询用户（按用户名）
func (u *users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	logger := u.getLogger(ctx)
	logger.Debug("存储层:开始查询用户信息")

	// 设置超时上下文
	dbCtx, cancel := u.createTimeoutContext(ctx)
	defer cancel()

	startTime := time.Now()

	var user *v1.User
	var err error

	// 使用自定义配置的retry（查询操作重试延迟更短）
	queryConfig := db.RetryConfig{
		MaxRetries:   2,
		InitialDelay: 50 * time.Millisecond,
		IsRetryable:  u.isRetryableError, // 使用自定义的重试判断
	}

	err = db.Do(dbCtx, queryConfig, func() error {
		user, err = u.executeSingleGet(dbCtx, username)
		return err
	})

	costMs := time.Since(startTime)
	//log.Warnf("stroe:err:%+v", err)
	if err != nil {
		return nil, u.handleGetError(err, username, logger, costMs)
	}

	u.logGetSuccess(user, logger, costMs)
	return user, nil
}

// createLogger 创建专用的日志实例
func (u *users) createLogger(ctx context.Context, user *v1.User) log.Logger {
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
func (u *users) getLogger(ctx context.Context) log.Logger {
	return log.L(ctx).WithValues(
		"layer", "store",
		"component", "users",
		"table", "user",
		"operation", "get",
	)
}

// createTimeoutContext 创建超时上下文
func (u *users) createTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
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
func (u *users) executeSingleGet(ctx context.Context, username string) (*v1.User, error) {
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
func (u *users) isRetryableError(err error) bool {
	// 首先使用默认判断
	if db.DefaultIsRetryable(err) {
		return true
	}

	// 可以在这里添加特定的重试逻辑
	// 例如：某些特定的业务错误也可以重试

	return false
}

// handleCreateError 处理创建错误
func (u *users) handleCreateError(err error, logger log.Logger, cost time.Duration) error {
	logger = logger.WithValues("cost_ms", cost.Milliseconds())

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		logger.Warn("用户创建操作超时", log.String("error", err.Error()))
		return errors.WithCode(code.ErrDatabaseTimeout, "创建用户超时")

	case u.isMySQLDuplicateError(err):
		logger.Info("创建用户失败：用户已存在", log.String("error", err.Error()))
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已存在")

	default:
		logger.Error("用户创建操作失败", log.String("error", err.Error()))
		return errors.WithCode(code.ErrDatabase, "数据库操作失败: %v", err)
	}
}

// handleGetError 处理查询错误
func (u *users) handleGetError(err error, username string, logger log.Logger, cost time.Duration) error {
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

// isMySQLDuplicateError 检查是否是MySQL唯一键冲突错误
func (u *users) isMySQLDuplicateError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}

// logCreateSuccess 记录创建成功日志
func (u *users) logCreateSuccess(logger log.Logger, cost time.Duration) {
	logger.Info("用户创建成功",
		//	log.String("username", user.Name),
		log.Int64("cost_ms", cost.Milliseconds()),
	)
}

// logGetSuccess 记录查询成功日志
func (u *users) logGetSuccess(user *v1.User, logger log.Logger, cost time.Duration) {
	logger.Info("用户查询成功",
		log.Int64("cost_ms", cost.Milliseconds()),
		log.Uint64("user_id", user.ID),
		log.Any("user_status", user.Status),
	)
}

func (u *users) executeInTransaction(ctx context.Context, logger log.Logger, fn func(tx *gorm.DB) error) error {
	txStartTime := time.Now()
	logger.Debugw("开始数据库事务", "tx_start_time", txStartTime)

	tx := u.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		txDuration := time.Since(txStartTime)
		logger.Errorw("开始事务失败", "error", tx.Error, "tx_duration_ms", txDuration.Milliseconds())
		return errors.WithCode(code.ErrDatabase, "开始事务失败")
	}

	var completed bool
	defer func() {
		if !completed {
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
		completed = false // 确保defer中的回滚会执行
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		logger.Errorw("提交事务失败", "error", err)
		completed = false // 确保回滚
		return errors.WithCode(code.ErrDatabase, "提交事务失败")
	}

	completed = true // 提交成功，不需要回滚
	logger.Debugw("事务提交成功")
	return nil
}
