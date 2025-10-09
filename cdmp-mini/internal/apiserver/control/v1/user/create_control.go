// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"context"
	"fmt"
	"strings"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Create(ctx *gin.Context) {
	// 从Gin上下文获取中间件设置的信息
	operator := common.GetUsername(ctx.Request.Context())
	auditBase := func(outcome, message string) {
		event := audit.BuildEventFromRequest(ctx.Request)
		event.Action = "user.create"
		event.ResourceType = "user"
		event.Actor = operator
		event.Outcome = outcome
		if message != "" {
			event.ErrorMessage = message
		}
		submitAudit(ctx, event)
	}
	var m metav1.CreateOptions
	if err := ctx.ShouldBindQuery(&m); err != nil {
		err := errors.WithCode(code.ErrBind, "传入的CreateOptions参数错误")
		core.WriteResponse(ctx, err, nil)
		auditBase("fail", err.Error())
		return
	}

	var r v1.User
	if err := ctx.ShouldBindJSON(&r); err != nil {
		log.Errorw("请求体绑定结构体失败", "requestID", ctx.Request.Header.Get("X-Request-ID"), "error", err)
		errBind := errors.WithCode(code.ErrBind, "参数绑定失败:%v", err.Error())
		core.WriteResponse(ctx, errBind, nil)
		auditBase("fail", errBind.Error())
		return
	}
	// 链路追踪日志
	log.Debugf("[control] 用户创建请求入口: username=%s, operator=%s", r.Name, operator)
	//校验用户名
	username := r.Name
	auditBase = func(outcome, message string) {
		event := audit.BuildEventFromRequest(ctx.Request)
		event.Action = "user.create"
		event.ResourceType = "user"
		event.Actor = operator
		event.ResourceID = username
		event.Outcome = outcome
		if message != "" {
			event.ErrorMessage = message
		}
		submitAudit(ctx, event)
	}
	if strings.TrimSpace(username) == "" {
		err := errors.WithCode(code.ErrValidation, "用户名不能为空")
		core.WriteResponse(ctx, err, nil)
		auditBase("fail", err.Error())
		return
	}
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errsMsg := strings.Join(errs, ":")
		log.Warnf("[control] 用户名不合法: username=%s, error=%s", username, errsMsg)
		err := errors.WithCode(code.ErrValidation, "用户名不合法:%s", errsMsg)
		core.WriteResponse(ctx, err, nil)
		auditBase("fail", err.Error())
		return
	}

	validationErrs := r.Validate()
	if len(validationErrs) > 0 {
		errDetails := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			errDetails[fieldErr.Field] = fieldErr.ErrorBody()
		}
		detailsStr := fmt.Sprintf("密码设定不符合规则: %+v", errDetails)
		log.Warnf("[control] 密码不符合规则: username=%s, detail=%s", username, detailsStr)
		err := errors.WrapC(nil, code.ErrValidation, "%s", detailsStr)
		core.WriteResponse(ctx, err, nil)
		auditBase("fail", err.Error())
		return
	}
	r.Password, _ = auth.Encrypt(r.Password)
	r.Status = 1
	r.LoginedAt = time.Now()

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

	if err := u.srv.Users().Create(c, &r,
		metav1.CreateOptions{}, u.options); err != nil {
		log.Errorf("[control] 用户创建 service 层失败: username=%s, error=%v", r.Name, err)
		core.WriteResponse(ctx, err, nil)
		auditBase("fail", err.Error())
		return
	}
	publicUser := v1.ConvertToPublicUser(&r)

	// 构建成功数据
	successData := gin.H{
		"create_user":    publicUser.Username,
		"operator":       operator,
		"operation_time": time.Now().Format(time.RFC3339),
		"operation_type": "create",
		"code":           code.ErrSuccess,
	}

	core.WriteResponse(ctx, nil, successData)
	auditBase("success", "")

}
