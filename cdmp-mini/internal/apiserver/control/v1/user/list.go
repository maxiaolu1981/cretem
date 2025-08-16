package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// List list the users in the storage.
// Only administrator can call this function.
func (u *UserController) List(c *gin.Context) {
	log.L(c).Info("control:服务员接到顾客需求,转身告诉后厨总调度,需要处理用户相关业务...")
	var r metav1.ListOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)
		return
	}
	//调用服务层进行处理
	users, err := u.srv.Users().List(c, r)
	log.Info("好的,我已经取到了菜")
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, users)
}
