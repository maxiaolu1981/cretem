package store

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

//store.UserStore
//这是一个接口（通常在 store 包中定义），声明了所有与 “用户” 相关的数据库操作方法（如查询、创建、更新用户等）

type UserStore interface {
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
}
