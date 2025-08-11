package user

import (
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// List list the users in the storage.
// Only administrator can call this function.
func (u *UserController) List(c *gin.Context) {
	log.L(c).Info("list user function called.")

	var r metav1.ListOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)

		return
	}

	users, err := u.srv.Users().List(c, r)
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, users)
}
