package user

import (
	srvv1 "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

// UserController create a user handler used to handle request for user resource.
// 服务员
type UserController struct {
	srv srvv1.Service
}

// NewUserController creates a user handler.

func NewUserController(store store.Factory) *UserController {
	log.Info("control:告诉厨房总厨仓库的地址")
	return &UserController{
		srv: srvv1.NewService(store),
	}
}
