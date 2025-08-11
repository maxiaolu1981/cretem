package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// DeleteCollection batch delete users by multiple usernames.
// Only administrator can call this function.
func (u *UserController) DeleteCollection(c *gin.Context) {
	log.L(c).Info("batch delete user function called.")

	usernames := c.QueryArray("name")

	if err := u.srv.Users().DeleteCollection(c, usernames, metav1.DeleteOptions{}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, nil)
}
