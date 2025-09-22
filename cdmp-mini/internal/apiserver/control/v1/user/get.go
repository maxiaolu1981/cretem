package user

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Get(ctx *gin.Context) {

	username := ctx.Param("name")
	var r metav1.GetOptions
	if err := ctx.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(ctx, errors.WithCode(code.ErrBind, "传入的GetOptions参数错误"), nil) // ErrBind - 400: 100003请求体绑定结构体失败
		return
	}

	log.L(ctx).WithValues(
		"controller", "UserController", // 标识当前控制器
		"action", "Get", // 标识当前操作
		"client_ip", ctx.ClientIP(), // 客户端IP
		"method", ctx.Request.Method, // 请求方法
		"kind", r.Kind,
		"apiVersion", r.APIVersion,
		"path", ctx.FullPath(), // 请求路径
		"resource_id", username,
		"user_agent", ctx.Request.UserAgent(),
	)

	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errMsg := strings.Join(errs, ":")
		log.Errorf("用户名参数校验失败:", "error", errMsg)
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "用户名不合法:%s", errMsg), nil)
		return
	}

	c := ctx.Request.Context()
	// 如果没有设置超时，添加默认超时
	// 使用HTTP请求的超时配置，而不是Redis超时
	if _, hasDeadline := c.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		// 使用ServerRunOptions中的请求超时时间
		requestTimeout := u.options.ServerRunOptions.CtxTimeout
		if requestTimeout == 0 {
			requestTimeout = 30 * time.Second // 默认30秒
		}
		c, cancel = context.WithTimeout(c, requestTimeout)
		defer cancel()
	}

	user, err := u.srv.Users().Get(c, username, metav1.GetOptions{}, u.options)
	if err != nil {
		//log.Errorf("查询用户%s失败,错误:%v", username, err.Error())
		core.WriteResponse(ctx, err, nil)
		return
	}
	if user == nil {
		// 用户不存在（业务正常状态）
		core.WriteResponse(ctx, errors.WithCode(code.ErrUserNotFound, "用户不存在"), nil)
		return
	}

	publicUser := v1.ConvertToPublicUser(user)
	log.Info("用户查询成功")
	core.WriteSuccessResponse(ctx, "查询用户详情成功", publicUser)
}
