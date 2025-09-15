package user

import (
	"context"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	// 使用辅助函数获取上下文值

	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Create",
	)
	logger.Debug("使用布隆过滤器检查用户名是否存在")
	if u.BloomFilter.TestString(user.Name) {
		// 布隆过滤器说可能存在，需要进一步数据库确认
		logger.Info("布隆过滤器提示用户名可能存在，进行数据库确认")
		_, err := u.Store.Users().Get(ctx, user.Name, metav1.GetOptions{})
		if err == nil {
			return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在%s", user.Name)
		}
		// 如果是其他错误，继续执行创建逻辑
	} else {
		// 布隆过滤器说肯定不存在，直接跳过数据库查询
		logger.Info("布隆过滤器确认用户名不存在，跳过数据库查询")
	}

	logger.Info("开始执行用户创建逻辑")

	// 执行数据库操作
	err := u.Store.Users().Create(ctx, user, opts)
	if err == nil {
		// 添加到布隆过滤器
		u.BloomFilter.AddString(user.Name)
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
