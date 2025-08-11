package policy

import (
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// List return all policies.
func (p *PolicyController) List(c *gin.Context) {
	log.L(c).Info("list policy function called.")

	var r metav1.ListOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, err.Error()), nil)

		return
	}

	policies, err := p.srv.Policies().List(c, c.GetString(middleware.UsernameKey), r)
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, policies)
}
