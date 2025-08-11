package policy

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// DeleteCollection delete policies by policy names.
func (p *PolicyController) DeleteCollection(c *gin.Context) {
	log.L(c).Info("batch delete policy function called.")

	if err := p.srv.Policies().DeleteCollection(c, c.GetString(middleware.UsernameKey),
		c.QueryArray("name"), metav1.DeleteOptions{}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, nil)
}
