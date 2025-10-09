package user

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) DeleteCollection(c *gin.Context) {

	usernames := c.QueryArray("name")
	operator := common.GetUsername(c.Request.Context())
	auditLog := func(outcome, message string) {
		event := audit.BuildEventFromRequest(c.Request)
		event.Action = "user.delete_collection"
		event.ResourceType = "user"
		event.ResourceID = fmt.Sprintf("batch:%d", len(usernames))
		event.Actor = operator
		event.Outcome = outcome
		if len(usernames) > 0 {
			event.Metadata = map[string]any{"usernames": usernames}
		}
		if message != "" {
			event.ErrorMessage = message
		}
		submitAudit(c, event)
	}
	if len(usernames) == 0 {
		err := fmt.Errorf("没有传入任何待删除的用户名")
		errWrap := errors.WithCode(code.ErrInvalidParameter, "%s", err.Error())
		core.WriteResponse(c, errWrap, nil)
		auditLog("fail", errWrap.Error())
		return
	}
	log.Infof("[control] 批量用户删除请求入口: usernames=%v", usernames)

	if err := u.srv.Users().DeleteCollection(c, usernames, true, metav1.DeleteOptions{}, u.options); err != nil {
		core.WriteResponse(c, err, nil)
		auditLog("fail", err.Error())
		return
	}

	core.WriteResponse(c, nil, nil)
	auditLog("success", "")
}
