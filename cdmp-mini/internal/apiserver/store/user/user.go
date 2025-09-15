package user

import (
	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

// 这个 users 结构体的设计与之前提到的 datastore 类似，但更聚焦于 “用户” 这一特定资源的数据库操作，通过持有 *gorm.DB 实例，专门封装与用户相关的数据库交互逻辑。
// 与 datastore 的区别：datastore 通常是全局或通用的数据库连接管理器，而 users 是更细分的 “用户资源操作类”，直接依赖 *gorm.DB 而非 datastore，结构更简洁。
// 仓库工人

// 作用：通过用户名（username）和查询选项（opts）从数据库中查询状态有效的用户，并返回符合条件的用户信息。
// 核心依赖：u.db（*gorm.DB）执行数据库查询，ctx（context.Context）用于传递上下文（如超时控制、追踪信息）。

type Users struct {
	db          *gorm.DB
	policyStore interfaces.PolicyStore
}

func NewUsers(db *gorm.DB, policyStore interfaces.PolicyStore) *Users {
	return &Users{
		db:          db,
		policyStore: policyStore,
	}
}

// isMySQLDuplicateError 检查是否是MySQL唯一键冲突错误
func (u *Users) isMySQLDuplicateError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 // MySQL重复键错误码
	}
	return false
}

// isMySQLDeadlockError 检查是否是MySQL死锁错误
func (u *Users) isMySQLDeadlockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213 // MySQL死锁错误码
	}
	return false
}

// isMySQLConnectionError 检查是否是MySQL连接错误（可选添加）
func (u *Users) isMySQLConnectionError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		// 常见的连接相关错误码
		switch mysqlErr.Number {
		case 2002, 2003, 2006, 2013:
			return true
		}
	}
	return false
}
