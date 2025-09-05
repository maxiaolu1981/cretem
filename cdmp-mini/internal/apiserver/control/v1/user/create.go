// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"strings"

	"time"

	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/core"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Create(ctx *gin.Context) {
	// 从Gin上下文获取中间件设置的信息

	logger := log.L(ctx).WithValues(
		"controller", "UserController",
		"action", "Create",
		"client_ip", ctx.ClientIP(), // 客户端IP
		"method", ctx.Request.Method, // 请求方法
		"path", ctx.FullPath(), // 请求路径 操作的资源ID
		"user_agent", ctx.Request.UserAgent(), // 用户代理
	)
	logger.Info("开始处理用户创建请求")

	var r v1.User

	if err := ctx.ShouldBindJSON(&r); err != nil {
		log.Errorw("请求体绑定结构体失败", "requestID", ctx.Request.Header.Get("X-Request-ID"), "error", err)
		core.WriteResponse(ctx, errors.WithCode(code.ErrBind, "参数绑定失败:%v", err.Error()), nil)
		return
	}
	//校验用户名
	username := r.Name
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errsMsg := strings.Join(errs, ":")
		log.Warnw("用户名不合法:", errsMsg)
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "用户名不合法:%s", errsMsg), nil)
		return
	}

	validationErrs := r.Validate()
	if len(validationErrs) > 0 {
		errDetails := make(map[string]string, len(validationErrs))
		for _, fieldErr := range validationErrs {
			errDetails[fieldErr.Field] = fieldErr.ErrorBody()
		}
		detailsStr := fmt.Sprintf("密码设定不符合规则: %+v", errDetails)
		err := errors.WrapC(
			nil,                // 无原始错误，创建全新带码错误
			code.ErrValidation, // 业务错误码
			"%s",
			detailsStr, // 错误消息（包含详情）
		)
		log.Warnw("密码生成不符合规则", detailsStr)
		core.WriteResponse(ctx, err, nil)
		return
	}
	r.Password, _ = auth.Encrypt(r.Password)
	r.Status = 1
	r.LoginedAt = time.Now()

	if err := u.srv.Users().Create(ctx, &r, metav1.CreateOptions{}); err != nil {
		core.WriteResponse(ctx, err, nil)
		return
	}
	// 返回时隐藏敏感信息
	responseUser := r
	responseUser.Password = ""
	core.CreateSuccessResponse(ctx, "用户创建成功", responseUser)
}
