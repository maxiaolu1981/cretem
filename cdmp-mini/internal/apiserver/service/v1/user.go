package v1

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type UserSrv interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) error
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User) error
}

type userService struct {
	store store.Factory
}

func newUsers(s *service) *userService {
	return &userService{
		store: s.store,
	}
}

func (u *userService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	return nil
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
func (u *userService) Get(ctx context.Context, username string, opts metav1.GetOptions) error {
	return nil
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
