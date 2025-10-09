// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
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

	metrics.MonitorBusinessOperation("user_service", "change_password", "http", func() error {
		var r ChangePasswordRequest

		if err := c.ShouldBindJSON(&r); err != nil {
			core.WriteResponse(c, errors.WithCode(code.ErrBind, "ChangePasswordRequest结构体转换错误%s", err.Error()), nil)
			return err
		}

		if strings.TrimSpace(r.OldPassword) == "" || strings.TrimSpace(r.NewPassword) == "" {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "%s", "旧密码和新密码不能为空"), nil)
			return errors.WithCode(code.ErrInvalidParameter, "旧密码和新密码不能为空")
		}

		if r.OldPassword == r.NewPassword {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "%s", "旧密码和新密码不能相同"), nil)
			return errors.WithCode(code.ErrInvalidParameter, "旧密码和新密码不能相同")
		}

		if err := validation.IsValidPassword(r.NewPassword); err != nil {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "%s", "新密码不符合要求"), nil)
			return errors.WithCode(code.ErrInvalidParameter, "新密码不符合要求")
		}

		user, err := u.srv.Users().Get(c, c.Param("name"), metav1.GetOptions{}, u.options)
		if err != nil {
			core.WriteResponse(c, err, nil)
			return err
		}

		if err := user.Compare(r.OldPassword); err != nil {
			core.WriteResponse(c, errors.WithCode(code.ErrPasswordIncorrect, "%s", err.Error()), nil)
			return errors.WithCode(code.ErrPasswordIncorrect,"%s", err.Error())
		}

		user.Password, _ = auth.Encrypt(r.NewPassword)
		if err := u.srv.Users().ChangePassword(c, user, u.options); err != nil {
			core.WriteResponse(c, err, nil)
			return err
		}

		core.WriteResponse(c, nil, nil)
		return nil
	})
}
