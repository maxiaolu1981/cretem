// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package secret

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

// Delete delete a secret by the secret identifier.
func (s *SecretController) Delete(c *gin.Context) {
	log.L(c).Info("delete secret function called.")
	opts := metav1.DeleteOptions{Unscoped: true}
	if err := s.srv.Secrets().Delete(c, c.GetString(middleware.UsernameKey), c.Param("name"), opts); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, nil)
}
