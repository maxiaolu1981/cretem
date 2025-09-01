// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Create(c *gin.Context) {
	logger := log.L(c)
	logger.Info("增在创建用户")

	var r v1.User

	if err := c.ShouldBindJSON(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%v", err.Error()), nil)
		return
	}
	if errs := r.Validate(); len(errs) != 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrValidation, "%v", errs.ToAggregate().Error()), nil)
		return
	}
	encryptedPassword, err := auth.Encrypt(r.Password)
	if err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrEncodingFailed, "%v", err.Error()), nil)
	}
	r.Password = encryptedPassword
	if r.Status == 0 {
		r.Status = 1
	}
	r.LoginedAt = time.Now()

	if err := u.srv.Users().Create(c, &r, metav1.CreateOptions{}); err != nil {
		core.WriteResponse(c, err, nil)
		return
	}
	// 返回时隐藏敏感信息
	responseUser := r
	r.Password = ""
	core.WriteResponse(c, nil, responseUser)
}
