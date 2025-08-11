// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"github.com/gin-gonic/gin"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// Update update a user info by the user identifier.
func (u *UserController) Update(c *gin.Context) {
	log.L(c).Info("update user function called.")

	var r v1.User

	if err := c.ShouldBindJSON(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)

		return
	}

	user, err := u.srv.Users().Get(c, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	user.Nickname = r.Nickname
	user.Email = r.Email
	user.Phone = r.Phone
	user.Extend = r.Extend

	if errs := user.ValidateUpdate(); len(errs) != 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrValidation, "%s", errs.ToAggregate().Error()), nil)

		return
	}

	// Save changed fields.
	if err := u.srv.Users().Update(c, user, metav1.UpdateOptions{}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, user)
}
