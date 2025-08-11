package policy

import (
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// Get return policy by the policy identifier.
func (p *PolicyController) Get(c *gin.Context) {
	log.L(c).Info("get policy function called.")

	pol, err := p.srv.Policies().Get(c, c.GetString(middleware.UsernameKey), c.Param("name"), metav1.GetOptions{})
	if err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, pol)
}
