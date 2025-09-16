package user

import (
	"context"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type UserService struct {
	Store       interfaces.Factory
	Redis       *storage.RedisCluster
	Options     *options.Options
	BloomFilter *bloom.BloomFilter
	BloomMutex  *sync.RWMutex
	Producer    interface{}
}

// NewUserService 创建用户服务实例
func NewUserService(store interfaces.Factory, redis *storage.RedisCluster, opts *options.Options) *UserService {
	return &UserService{
		Store:   store,
		Redis:   redis,
		Options: opts,
	}
}

type UserSrv interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User) error
}

// 业务方法：检查用户名是否可能存在
func (us *UserService) UsernameMightExist(username string) bool {
	// 使用字符串专用的便捷方法
	return us.BloomFilter.TestString(username)
}
