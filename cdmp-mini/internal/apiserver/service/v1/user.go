package v1

import (
	"context"

	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

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
}

func newUsers(s *service) *userService {
	return &userService{
		store: s.store,
	}
}

func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	// 使用辅助函数获取上下文值
	requestID := common.GetRequestID(ctx)
	operator := common.GetUsername(ctx)

	// 合并所有字段，只声明一次 logger
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"operation", "create_user",
		"request_id", requestID,
		"operator", operator,
		"target_user", user.Name,
		"user_id", user.ID,
		"user_status", user.Status,
	)

	logger.Debug("开始执行用户创建逻辑")

	// 执行数据库操作
	err := u.store.Users().Create(ctx, user, opts)
	if err == nil {
		logger.Debug("用户创建成功")
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
	return nil
}
func (u *userService) DeleteCollection(ctx context.Context, username []string, opts metav1.DeleteOptions) error {
	return nil
}
func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
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

// 是UserSrv接口的具体实现,专门处理用户相关业务,比如查询用户列表等.
// 实现具体的业务逻辑 直接调用store 获取或者修改数据 就像厨师团队从仓库拿食材
