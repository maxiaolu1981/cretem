package user

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sru "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/service/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Get(ctx *gin.Context) {
	log.Debug("control:开始处理用户查询请求...")

	operator := common.GetUsername(ctx.Request.Context())
	username := ctx.Param("name")
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errMsg := strings.Join(errs, ":")
		log.Errorf("用户名参数校验失败:", "error", errMsg)
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "用户名不合法:%s", errMsg), nil)
		return
	}

	c := ctx.Request.Context()
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

	//从服务层查询用户
	user, err := u.srv.Users().Get(c, username, metav1.GetOptions{}, u.options)
	//数据库错误
	if err != nil {
		core.WriteResponse(ctx, err, nil)
		return
	}
	// 用户不存在（业务正常状态）
	if user.Name == sru.RATE_LIMIT_PREVENTION {
		err := errors.WithCode(code.ErrPasswordIncorrect, "用户名密码无效")
		core.WriteResponse(ctx, err, nil)
		return
	}

	publicUser := v1.ConvertToPublicUser(user)
	// 构建成功数据
	successData := gin.H{
		"get":            publicUser.Username,
		"operator":       operator,
		"operation_time": time.Now().Format(time.RFC3339),
		"operation_type": "create",
	}
	core.WriteResponse(ctx, nil, successData)
}
