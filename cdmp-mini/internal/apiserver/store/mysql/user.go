package mysql

import (
	"context"

	gormutil "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/fields"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

// 这个 users 结构体的设计与之前提到的 datastore 类似，但更聚焦于 “用户” 这一特定资源的数据库操作，通过持有 *gorm.DB 实例，专门封装与用户相关的数据库交互逻辑。
// 与 datastore 的区别：datastore 通常是全局或通用的数据库连接管理器，而 users 是更细分的 “用户资源操作类”，直接依赖 *gorm.DB 而非 datastore，结构更简洁。
// 仓库工人
type users struct {
	db *gorm.DB
}

// 作用：通过用户名（username）和查询选项（opts）从数据库中查询状态有效的用户，并返回符合条件的用户信息。
// 核心依赖：u.db（*gorm.DB）执行数据库查询，ctx（context.Context）用于传递上下文（如超时控制、追踪信息）。
func (u *users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	// // 校验资源类型是否为 "User"
	// if opts.Kind != "User" {
	// 	return nil, errors.WithCode(code.ErrInvalidResourceKind, "资源类型不匹配,期望:User,实际:%s", opts.Kind)
	// }
	// // 校验 API 版本是否为 "v1"
	// if opts.APIVersion != "v1" {
	// 	return nil, errors.WithCode(code.ErrInvalidAPIVersion, "API版本不支持,期望:v1,实际:%s", opts.APIVersion)
	// }
	// 初始化用户对象（v1.User 是定义用户结构的模型，如包含 ID、Name、Status 等字段）
	user := &v1.User{}
	err := u.db.Where("name=? and status =1", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "%s", err.Error())
		}
		return nil, errors.WithCode(code.ErrDatabase, "%s", err.Error())
	}
	return user, nil
}

// 这个 newUsers 函数是 users 结构体的构造函数，作用是基于已有的 datastore 实例创建 users 实例，实现了 datastore 与 users 之间的依赖传递，是典型的 “组合复用” 设计。
// 通过 datastore 这个 “数据库连接管理器”，为 users 提供已初始化的 *gorm.DB 实例，使 users 可以专注于实现用户相关的数据库操作（如 GetByID、Create 等），而无需关心数据库连接的创建细节。
func newUsers(ds *datastore) *users {
	log.Info("mysql:仓库保管员说:好的,我知道了..")
	return &users{ds.db}
}

func (u *users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return u.db.Save(user).Error
}

// List return all users.
func (u *users) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	log.Info("store:好的 我来给你所需要的食材")
	ret := &v1.UserList{}
	ol := gormutil.Unpointer(opts.Offset, opts.Limit)

	selector, _ := fields.ParseSelector(opts.FieldSelector)
	username, _ := selector.RequiresExactMatch("name")
	d := u.db.Where("name like ? and status = 1", "%"+username+"%").
		Offset(ol.Offset).
		Limit(ol.Limit).
		Order("id desc").
		Find(&ret.Items).
		Offset(-1).
		Limit(-1).
		Count(&ret.TotalCount)

	return ret, d.Error
}
