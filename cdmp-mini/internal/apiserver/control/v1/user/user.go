package user

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	service "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
)

type UserController struct {
	srv      service.ServiceManager
	options  *options.Options
	Producer producer.MessageProducer
}

// NewUserController creates a user handler.
func NewUserController(store interfaces.Factory,
	redis *storage.RedisCluster,
	options *options.Options,
	bloomFilter *bloom.BloomFilter,
	bloomMutex *sync.RWMutex, producer producer.MessageProducer) (*UserController, error) {

	s, err := service.NewService(store,
		redis, options,
		bloomFilter, bloomMutex, producer)
	if err != nil {
		return nil, err
	}
	return &UserController{
		srv:      s,
		options:  options,
		Producer: producer,
	}, nil
}

// BusinessValidateListOptions 业务层验证函数
func (u *UserController) validateListOptions(opts *v1.ListOptions) field.ErrorList {
	if *opts.TimeoutSeconds == 0 {
		log.Warnw("目前查询模式是非超时模式")
	}
	return validation.ValidateListOptionsBase(opts)
}

func (u *UserController) validateGetOptions(opts *metav1.GetOptions) field.ErrorList {
	return validation.ValidateGetOptionsBase(opts)
}

func (u *UserController) validateCreateOptions(opts *metav1.CreateOptions) field.ErrorList {
	return validation.ValidateCreateOptionsBase(opts)
}
