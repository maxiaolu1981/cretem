package user

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	service "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
)

type UserController struct {
	srv     service.Service
	options *options.Options
}

// NewUserController creates a user handler.
func NewUserController(store store.Factory, redis *storage.RedisCluster, options *options.Options) *UserController {

	return &UserController{
		srv:     service.NewService(store, redis, options),
		options: options,
	}
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
