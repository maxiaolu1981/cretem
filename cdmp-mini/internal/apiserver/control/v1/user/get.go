package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *UserController) Get(c *gin.Context) {

	logger := log.L(c).WithValues(
		"controller", "UserController", // 标识当前控制器
		"action", "Get", // 标识当前操作
		"client_ip", c.ClientIP(), // 客户端IP
		"method", c.Request.Method, // 请求方法
		"path", c.FullPath(), // 请求路径
		"resource_id", c.Param("id"),
		"user_agent", c.Request.UserAgent(),
	)
	logger.Info("开始处理用户创建请求")
	user, err := u.srv.Users().Get(c, c.Param("name"), metav1.GetOptions{})
	if err != nil {
		core.WriteResponse(c, err, nil)
		return
	}
	core.WriteResponse(c, nil, user)
}
