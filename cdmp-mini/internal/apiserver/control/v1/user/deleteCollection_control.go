package user

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) DeleteCollection(c *gin.Context) {

	usernames := c.QueryArray("name")
	if len(usernames) == 0 {
		err := fmt.Errorf("没有传入任何待删除的用户名")
		core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "%s",err.Error()), nil)
		return
	}
	log.Infof("[control] 批量用户删除请求入口: usernames=%v", usernames)

	if err := u.srv.Users().DeleteCollection(c, usernames, true, metav1.DeleteOptions{}, u.options); err != nil {
		core.WriteResponse(c, err, nil)
		return
	}

	core.WriteResponse(c, nil, nil)
}
