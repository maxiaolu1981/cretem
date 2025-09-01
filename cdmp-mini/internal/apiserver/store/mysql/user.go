package mysql

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
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
		db: ds.db,
	}
}

func (u *users) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	return u.db.Create(&user).Error
}

func (u *users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return nil
}

func (u *users) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	return nil
}

func (u *users) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error {
	return nil
}

func (u *users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	return nil, nil
}

func (u *users) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	return nil, nil
}
