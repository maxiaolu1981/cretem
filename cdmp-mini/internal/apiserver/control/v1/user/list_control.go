package user

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) List(c *gin.Context) {

	log.Debugf("开始处理list请求...")
	operator := common.GetUsername(c.Request.Context())
	auditLog := func(outcome string, err error, meta map[string]any) {
		event := audit.BuildEventFromRequest(c.Request)
		event.Action = "user.list"
		event.ResourceType = "user"
		event.ResourceID = "collection"
		event.Actor = operator
		event.Outcome = outcome
		if err != nil {
			event.ErrorMessage = err.Error()
		}
		if len(meta) > 0 {
			event.Metadata = meta
		}
		submitAudit(c, event)
	}
	metrics.MonitorBusinessOperation("user_service", "list", "http", func() error {

		var r metav1.ListOptions
		if err := c.ShouldBindQuery(&r); err != nil {
			errWrap := errors.WithCode(code.ErrBind, "传入的参数错误")
			core.WriteResponse(c, errWrap, nil) // ErrBind - 400: 100003请求体绑定结构体失败
			auditLog("fail", errWrap, map[string]any{"stage": "bind"})
			return errWrap
		}

		errs := u.validateListOptions(&r)
		if len(errs) > 0 {
			errDetails := make(map[string]string, len(errs))
			for _, fieldErr := range errs {
				errDetails[fieldErr.Field] = fieldErr.ErrorBody()
			}
			detailStr := fmt.Sprintf("参数错误:%+v", errDetails)
			err := errors.WrapC(nil, code.ErrInvalidParameter, "%s", detailStr)
			core.WriteResponse(c, err, nil)
			auditLog("fail", err, map[string]any{"stage": "validate"})
			return err
		}

		userList, err := u.srv.Users().List(c, r, u.options)
		if err != nil {
			errWrap := errors.WrapC(err, code.ErrInternal, "%s", errors.GetMessage(err))
			core.WriteResponse(c, errWrap, nil)
			auditLog("fail", errWrap, map[string]any{"stage": "service"})
			return errWrap
		}

		var publicUser *v1.PublicUser
		var publicUsers []*v1.PublicUser

		if len(userList.Items) > 0 {
			for _, u := range userList.Items {
				publicUser = v1.ConvertToPublicUser(u)
				publicUsers = append(publicUsers, publicUser)
			}

		}
		core.WriteResponse(c, nil, publicUsers)
		meta := map[string]any{
			"returned_count": len(publicUsers),
		}
		if r.Limit != nil {
			meta["limit"] = *r.Limit
		}
		if r.Offset != nil {
			meta["offset"] = *r.Offset
		}
		if r.LabelSelector != "" {
			meta["label_selector"] = r.LabelSelector
		}
		if r.FieldSelector != "" {
			meta["field_selector"] = r.FieldSelector
		}
		auditLog("success", nil, meta)
		return nil
	})
}
