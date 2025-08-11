package policy

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

// Delete deletes the policy by the policy identifier.
func (p *PolicyController) Delete(c *gin.Context) {
	log.Info("delete policy function called.")

	if err := p.srv.Policies().Delete(c, c.GetString(middleware.UsernameKey), c.Param("name"),
		metav1.DeleteOptions{}); err != nil {
		core.WriteResponse(c, err, nil)

		return
	}

	core.WriteResponse(c, nil, nil)
}
