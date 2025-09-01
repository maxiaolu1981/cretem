package user

import (
	service "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
)

type UserController struct {
	srv service.Service
}

// NewUserController creates a user handler.
func NewUserController(store store.Factory) *UserController {
	return &UserController{
		srv: service.NewService(store),
	}
}
