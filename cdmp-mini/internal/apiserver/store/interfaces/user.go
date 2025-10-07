package interfaces

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type UserStore interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions, opt *options.Options) error
	DeleteForce(ctx context.Context, username string, opts metav1.DeleteOptions, opt *options.Options) error
	DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions, opt *options.Options) error
	Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error)
	List(ctx context.Context, username string, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error)
	ListAllUsernames(ctx context.Context) ([]string, error)
	ListAll(ctx context.Context, username string) (*v1.UserList, error)
}
