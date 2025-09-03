package v1

import (
	"context"
	"time"

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
)

type UserSrv interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User) error
}

var _ UserSrv = &userService{}

type userService struct {
	store store.Factory
	redis *storage.RedisCluster
}

func newUsers(s *service) *userService {
	return &userService{
		store: s.store,
		redis: s.redis,
	}
}

func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	// 使用辅助函数获取上下文值

	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Create",
	)

	logger.Info("开始执行用户创建逻辑")

	// 执行数据库操作
	err := u.store.Users().Create(ctx, user, opts)
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
func (u *userService) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Delete",
	)

	logger.Info("开始执行用户删除操作")
	startTime := time.Now()
	defer func() {
		logger.WithValues("cost_ms", time.Since(startTime).Microseconds()).Info("用户删除结束")
	}()
	//参数校验
	if username == "" {
		err := errors.WithCode(code.ErrInvalidParameter, "用户名不能为空")
		logger.Errorw("删除失败:参数无效", "error", err)
		return err
	}
	//并发控制
	lock := lock.NewRedisLock(u.redis, "user:delete:"+username, 30*time.Second)

	acquired, err := lock.TryAcquire(ctx, 3, 100*time.Microsecond)
	if err != nil {
		logger.Errorw("获取分布锁失败", "error", err, "lock_err", lock.GetKey())
		return errors.WithCode(code.ErrInternal, "系统繁忙,请稍后再试")
	}
	if !acquired {
		logger.Warnw("获取分布式锁失败，可能其他进程正在操作同一用户", "lock_key", lock.GetKey())
		return errors.WithCode(code.ErrResourceConflict, "用户正在被其他操作处理，请稍后重试")
	}
	// 确保释放锁
	defer func() {
		if releaseErr := lock.Release(ctx); releaseErr != nil {
			logger.Errorw("释放分布式锁失败", "error", releaseErr, "lock_key", lock.GetKey())
		}
	}()

	logger.Infow("成功获取分布式锁，开始执行删除操作", "lock_key", lock.GetKey())

	err = u.store.Users().Delete(ctx, username, metav1.DeleteOptions{})
	if err != nil {
		logger.Info("删除失败")
		return err
	}
	return nil
}
func (u *userService) DeleteCollection(ctx context.Context, username []string, opts metav1.DeleteOptions) error {
	return nil
}
func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	// 使用辅助函数获取上下文值
	//requestID := common.GetRequestID(ctx)
	//operator := common.GetUsername(ctx)

	// 合并所有字段，只声明一次 logger
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"operation", "Get",
	)
	logger.Info("开始执行用户查询逻辑")

	user, err := u.store.Users().Get(ctx, username, opts)
	if err != nil {
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
