package user

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Get(c *gin.Context) {

	username := c.Param("name")

	logger := log.L(c).WithValues(
		"controller", "UserController", // 标识当前控制器
		"action", "Get", // 标识当前操作
		"client_ip", c.ClientIP(), // 客户端IP
		"method", c.Request.Method, // 请求方法
		"path", c.FullPath(), // 请求路径
		"resource_id", username,
		"user_agent", c.Request.UserAgent(),
	)
	logger.Info("开始处理用户查询请求(单资源)")
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errMsg := strings.Join(errs, ":")
		log.Warnw("用户名参数校验失败:", "error", errMsg)
		core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "用户名不合法:%s", errMsg), nil)
		return
	}
	user, err := u.srv.Users().Get(c, username, metav1.GetOptions{})
	if err != nil {
		log.Errorw("查询用户失败", "username:", username, "error:", err.Error())
		core.WriteResponse(c, err, nil)
		return
	}
	publicUser := v1.ConvertToPublicUser(user)
	core.WriteSuccessResponse(c, "查询用户详情成功", publicUser)
}
