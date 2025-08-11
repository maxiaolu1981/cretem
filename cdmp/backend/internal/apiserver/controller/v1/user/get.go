// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"

	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Get get an user by the user identifier.
func (u *UserController) Get(c *gin.Context) {
	log.L(c).Info("get user function called.")

	user, err := u.srv.Users().Get(c, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, user)
}
