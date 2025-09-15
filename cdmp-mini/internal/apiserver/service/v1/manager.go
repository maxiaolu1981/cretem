package v1

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"

	policy "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/policy"
	secret "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/secret"
	user "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/user"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
)

type ServiceSrv struct {
	Store       interfaces.Factory
	Redis       *storage.RedisCluster
	Options     *options.Options
	BloomFilter *bloom.BloomFilter
	BloomMutex  *sync.RWMutex
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
		Store:       s.Store,
		Redis:       s.Redis,
		Options:     s.Options,
		BloomFilter: s.BloomFilter,
		BloomMutex:  s.BloomMutex,
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

func NewService(store interfaces.Factory,
	redis *storage.RedisCluster,
	options *options.Options,
	bloomFilter *bloom.BloomFilter,
	bloomMutex *sync.RWMutex) (ServiceManager, error) {
	// 初始化布隆过滤器（根据预期用户数量配置）
	s := &ServiceSrv{
		Store:       store,
		Redis:       redis,
		Options:     options,
		BloomFilter: bloomFilter,
		BloomMutex:  bloomMutex,
	}

	return s, nil
}
