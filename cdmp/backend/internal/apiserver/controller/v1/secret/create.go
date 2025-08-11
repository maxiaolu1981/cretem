// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package secret

import (
	"github.com/AlekSi/pointer"
	"github.com/gin-gonic/gin"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/idutil"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

const maxSecretCount = 10

// Create add new secret key pairs to the storage.
func (s *SecretController) Create(c *gin.Context) {
	log.L(c).Info("create secret function called.")

	var r v1.Secret

	if err := c.ShouldBindJSON(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)

		return
	}

	if errs := r.Validate(); len(errs) != 0 {
		core.WriteResponse(c, errors.WithCode(code.ErrValidation, "%s", errs.ToAggregate().Error()), nil)

		return
	}

	username := c.GetString(middleware.UsernameKey)

	secrets, err := s.srv.Secrets().List(c, username, metav1.ListOptions{
		Offset: pointer.ToInt64(0),
		Limit:  pointer.ToInt64(-1),
	})
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	if secrets.TotalCount >= maxSecretCount {
		core.WriteResponse(c, errors.WithCode(code.ErrReachMaxCount, "secret count: %d", secrets.TotalCount), nil)

		return
	}

	// must reassign username
	r.Username = username

	// generate secret id and secret key
	r.SecretID = idutil.NewSecretID()
	r.SecretKey = idutil.NewSecretKey()

	if err := s.srv.Secrets().Create(c, &r, metav1.CreateOptions{}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, r)
}
