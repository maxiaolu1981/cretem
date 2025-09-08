/*
组件	饭店比喻	职责
Factory接口	仓库总调度系统	定义仓库访问标准
mysqlFactory	智能自动化仓库	MySQL数据库实现
sqlserverFactory	传统大型仓库	SQL Server实现
redisFactory	快速冷藏库	Redis缓存实现
Client()	调度热线电话	获取当前仓库系统
SetClient()	更换仓库供应商	设置具体仓库实现
*/

package store

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

var client Factory

type Factory interface {
	Users() UserStore
	Secrets() SecretStore
	Polices() PolicyStore
	PolicyAudits() PolicyAuditStore
	Close() error
}

func Client() Factory {
	return client
}
func SetClient(factory Factory) {
	client = factory
}

// UserStore defines the user storage interface.
type UserStore interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error
	DeleteForce(ctx context.Context, username string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
}

type SecretStore interface {
	Create(ctx context.Context, secret *v1.Secret, opts metav1.CreateOptions) error
	Update(ctx context.Context, secret *v1.Secret, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username, secretID string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username string, secretIDs []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username, secretID string, opts metav1.GetOptions) (*v1.Secret, error)
	List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.SecretList, error)
}

type PolicyStore interface {
	Create(ctx context.Context, policy *v1.Policy, opts metav1.CreateOptions) error
	Update(ctx context.Context, policy *v1.Policy, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username string, names []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, name string, opts metav1.GetOptions) (*v1.Policy, error)
	List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.PolicyList, error)
}

type PolicyAuditStore interface {
	ClearOutdated(ctx context.Context, maxReserveDays int) (int64, error)
}
