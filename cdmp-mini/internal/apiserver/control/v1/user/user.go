package user

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	service "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
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
