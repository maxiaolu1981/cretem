package v1

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"

	policy "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/policy"
	secret "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/secret"
	user "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
)

type ServiceSrv struct {
	Store   store.Factory
	Redis   *storage.RedisCluster
	Options *options.Options
}

type ServiceManager interface {
	Users() user.UserSrv
	Secrets() secret.SecretSrv
	Policies() policy.PolicySrv
}

func (s *ServiceSrv) Users() user.UserSrv {
	return NewUsers(s)
}

func (s *ServiceSrv) Secrets() secret.SecretSrv {
	return NewSecrets(s)
}

func (s *ServiceSrv) Policies() policy.PolicySrv {
	return NewPolicies(s)
}

func NewUsers(s *ServiceSrv) *user.UserService {
	return &user.UserService{
		Store:   s.Store,
		Redis:   s.Redis,
		Options: s.Options,
	}
}
func NewSecrets(s *ServiceSrv) *secret.SecretService {
	return &secret.SecretService{
		Store:   s.Store,
		Redis:   s.Redis,
		Options: s.Options,
	}
}

func NewPolicies(s *ServiceSrv) *policy.PolicService {
	return &policy.PolicService{
		Store:   s.Store,
		Redis:   s.Redis,
		Options: s.Options,
	}
}

func NewService(store store.Factory, redis *storage.RedisCluster, options *options.Options) ServiceManager {
	return &ServiceSrv{
		Store:   store,
		Redis:   redis,
		Options: options,
	}
}
