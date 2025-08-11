// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package secret

import (
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// List list all the secrets.
func (s *SecretController) List(c *gin.Context) {
	log.L(c).Info("list secret function called.")
	var r metav1.ListOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, err.Error()), nil)

		return
	}

	secrets, err := s.srv.Secrets().List(c, c.GetString(middleware.UsernameKey), r)
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, secrets)
}
