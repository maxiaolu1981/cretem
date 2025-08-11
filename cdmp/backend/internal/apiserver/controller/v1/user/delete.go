package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Delete delete an user by the user identifier.
// Only administrator can call this function.
func (u *UserController) Delete(c *gin.Context) {
	log.L(c).Info("delete user function called.")

	if err := u.srv.Users().Delete(c, c.Param("name"), metav1.DeleteOptions{Unscoped: true}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, nil)
}
