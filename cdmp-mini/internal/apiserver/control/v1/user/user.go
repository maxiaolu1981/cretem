package user

import (
	service "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
)

// 服务员
type UserController struct {
	srv service.Service
}

// 服务员技能培训
func NewUserController(store store.Factory) *UserController {
	return &UserController{
		srv: service.NewService(store),
	}
}
