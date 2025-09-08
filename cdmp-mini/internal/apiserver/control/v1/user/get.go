package user

import (
	"fmt"
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
	var r metav1.GetOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "传入的GetOptions参数错误"), nil) // ErrBind - 400: 100003请求体绑定结构体失败
		return
	}
	if r.Kind == "" {
		r.Kind = "GetUserOption"
	}
	if r.APIVersion == "" {
		r.APIVersion = u.options.MetaOptions.GetOptions.APIVersion
	}

	logger := log.L(c).WithValues(
		"controller", "UserController", // 标识当前控制器
		"action", "Get", // 标识当前操作
		"client_ip", c.ClientIP(), // 客户端IP
		"method", c.Request.Method, // 请求方法
		"kind", r.Kind,
		"apiVersion", r.APIVersion,
		"path", c.FullPath(), // 请求路径
		"resource_id", username,
		"user_agent", c.Request.UserAgent(),
	)
	logger.Debugf("开始处理用户查询请求(单资源)")
	errs := u.validateGetOptions(&r)
	if len(errs) > 0 {
		errDetails := make(map[string]string, len(errs))
		for _, fieldErr := range errs {
			errDetails[fieldErr.Field] = fieldErr.ErrorBody()
		}
		detailStr := fmt.Sprintf("参数错误:%+v", errDetails)
		err := errors.WrapC(nil, code.ErrInvalidParameter, "%s", detailStr)
		core.WriteResponse(c, err, nil)
		return
	}

	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errMsg := strings.Join(errs, ":")
		log.Warnw("用户名参数校验失败:", "error", errMsg)
		core.WriteResponse(c, errors.WithCode(code.ErrValidation, "用户名不合法:%s", errMsg), nil)
		return
	}
	user, err := u.srv.Users().Get(c, username, metav1.GetOptions{})
	if err != nil {
		log.Debugw("查询用户失败", "username:", username, "error:", err.Error())

		coder := errors.ParseCoderByErr(err)
		log.Debugf("cotrol:返回的业务码%v", coder.Code())
		core.WriteResponse(c, err, nil)
		return
	}
	publicUser := v1.ConvertToPublicUser(user)
	log.Info("用户查询成功")
	core.WriteSuccessResponse(c, "查询用户详情成功", publicUser)
}
