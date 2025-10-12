package user

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Delete(ctx *gin.Context) {

	// deleteUsername := ctx.Param("name")
	// if deleteUsername == "" {
	// 	log.Error("要删除的用户名为空")
	// 	core.WriteResponse(ctx, errors.WithCode(code.ErrUserNotFound, "要删除的用户名为空"), nil)
	// 	return
	// }

	// _, err := interfaces.Client().Users().Get(stdCtx, deleteUsername, metav1.GetOptions{}, u.options)
	// if err != nil {
	// 	// 关键：匹配 Get 方法返回的 "用户不存在" 错误码 code.ErrUserNotFound
	// 	if errors.IsCode(err, code.ErrUserNotFound) {
	// 		log.Info("用户不存在，无需删除")
	// 		core.WriteResponse(ctx, nil, "用户不存在，无需删除")
	// 		return
	// 	}
	// 	// 其他错误（如超时、数据库异常）
	// 	logger.Errorf("查询用户信息失败%s", err)
	// 	core.WriteResponse(ctx, err, nil) // 直接返回 Get 方法处理后的错误（已包含错误码）
	// 	return
	//	}

	// if err := u.srv.Users().Delete(stdCtx, deleteUsername, false, metav1.DeleteOptions{Unscoped: false}, u.options); err != nil {
	// 	logger.Errorf("用户删除失败%v", err)
	// 	core.WriteResponse(ctx, err, nil)
	// 	return
	// }
	// logger.Info("用户删除成功")
	//core.WriteResponse(ctx, nil, nil)
}

func (u *UserController) ForceDelete(ctx *gin.Context) {

	// 校验：提取并检查待删除用户名（参数无效场景）
	operator := common.GetUsername(ctx.Request.Context())
	deleteUsername := ctx.Param("name")
	auditLog := func(outcome, message string) {
		event := audit.BuildEventFromRequest(ctx.Request)
		event.Action = "user.delete"
		event.ResourceType = "user"
		event.ResourceID = deleteUsername
		event.Actor = operator
		event.Outcome = outcome
		if message != "" {
			event.ErrorMessage = message
		}
		submitAudit(ctx, event)
	}
	metrics.MonitorBusinessOperation("user_service", "get", "http", func() error {
		if errs := validation.IsQualifiedName(deleteUsername); len(errs) > 0 {
			errsMsg := strings.Join(errs, ":")
			log.Warnf("[control] 用户名不合法: username=%s, error=%s", deleteUsername, errsMsg)
			err := errors.WithCode(code.ErrInvalidParameter, "用户名不合法:%s", errsMsg)
			core.WriteResponse(ctx, err, nil)
			auditLog("fail", err.Error())
			return err
		}

		c := ctx.Request.Context()
		// 使用HTTP请求的超时配置，而不是Redis超时
		if _, hasDeadline := c.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			// 使用ServerRunOptions中的请求超时时间
			requestTimeout := u.options.ServerRunOptions.CtxTimeout
			if requestTimeout == 0 {
				requestTimeout = 30 * time.Second // 默认30秒
			}
			c, cancel = context.WithTimeout(c, requestTimeout)
			defer cancel()
		}

		rawDelErr := u.srv.Users().Delete(
			c,
			deleteUsername,
			true, // force=true：强制删除
			metav1.DeleteOptions{Unscoped: true},
			u.options,
		)
		if rawDelErr != nil {
			log.Errorf("[control] 用户强制删除 service 层失败: username=%s, error=%v", deleteUsername, rawDelErr)
			err := errors.WrapC(
				rawDelErr,
				code.ErrInternalServer,
				"用户[%s]强制删除失败，请稍后重试",
				deleteUsername,
			)
			core.WriteResponse(ctx, err, nil)
			auditLog("fail", err.Error())
			return err
		}

		//成功场景：返回 RESTful 标准 204 No Content（无响应体）

		// 构建成功数据
		successData := gin.H{
			"delete_user":    deleteUsername,
			"operator":       operator,
			"operation_time": time.Now().Format(time.RFC3339),
			"operation_type": "delete",
		}

		core.WriteResponse(ctx, nil, successData)
		auditLog("success", "")
		return nil
	})
}
