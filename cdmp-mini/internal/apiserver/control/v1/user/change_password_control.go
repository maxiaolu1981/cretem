// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// ChangePasswordRequest defines the ChangePasswordRequest data format.
type ChangePasswordRequest struct {
	// Old password.
	// Required: true
	OldPassword string `json:"oldPassword" binding:"omitempty"`

	// New password.
	// Required: true
	NewPassword string `json:"newPassword" binding:"password"`
}

// ChangePassword change the user's password by the user identifier.
func (u *UserController) ChangePassword(c *gin.Context) {
	log.L(c).Info("开始进行密码修改.")
	operator := common.GetUsername(c.Request.Context())
	username := c.Param("name")
	auditLog := func(outcome, message string) {
		event := audit.BuildEventFromRequest(c.Request)
		event.Action = "user.change_password"
		event.ResourceType = "user"
		event.ResourceID = username
		event.Actor = operator
		event.Outcome = outcome
		if message != "" {
			event.ErrorMessage = message
		}
		submitAudit(c, event)
	}

	metrics.MonitorBusinessOperation("user_service", "change_password", "http", func() error {
		var r ChangePasswordRequest

		if err := c.ShouldBindJSON(&r); err != nil {
			errBind := errors.WithCode(code.ErrBind, "ChangePasswordRequest结构体转换错误%s", err.Error())
			core.WriteResponse(c, errBind, nil)
			auditLog("fail", errBind.Error())
			return errBind
		}

		if strings.TrimSpace(r.OldPassword) == "" || strings.TrimSpace(r.NewPassword) == "" {
			errEmpty := errors.WithCode(code.ErrInvalidParameter, "%s", "旧密码和新密码不能为空")
			core.WriteResponse(c, errEmpty, nil)
			auditLog("fail", errEmpty.Error())
			return errEmpty
		}

		if r.OldPassword == r.NewPassword {
			errSame := errors.WithCode(code.ErrInvalidParameter, "%s", "旧密码和新密码不能相同")
			core.WriteResponse(c, errSame, nil)
			auditLog("fail", errSame.Error())
			return errSame
		}

		if err := validation.IsValidPassword(r.NewPassword); err != nil {
			errInvalid := errors.WithCode(code.ErrInvalidParameter, "%s", "新密码不符合要求")
			core.WriteResponse(c, errInvalid, nil)
			auditLog("fail", errInvalid.Error())
			return errInvalid
		}

		user, err := u.srv.Users().Get(c, c.Param("name"), metav1.GetOptions{}, u.options)
		if err != nil {
			core.WriteResponse(c, err, nil)
			auditLog("fail", err.Error())
			return err
		}

		if err := user.Compare(r.OldPassword); err != nil {
			errCompare := errors.WithCode(code.ErrPasswordIncorrect, "%s", err.Error())
			core.WriteResponse(c, errCompare, nil)
			auditLog("fail", errCompare.Error())
			return errCompare
		}

		user.Password, _ = auth.Encrypt(r.NewPassword)
		if err := u.srv.Users().ChangePassword(c, user, u.options); err != nil {
			core.WriteResponse(c, err, nil)
			auditLog("fail", err.Error())
			return err
		}

		core.WriteResponse(c, nil, nil)
		auditLog("success", "")
		return nil
	})
}
