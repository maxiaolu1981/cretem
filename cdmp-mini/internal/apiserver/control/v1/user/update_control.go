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

func (u *UserController) Update(ctx *gin.Context) {
	username := ctx.Param("name") // 从URL路径获取，清晰明确
	log.Infof("[control] 用户更新请求入口: username=%s", username)

	var r v1.User
	if err := ctx.ShouldBindJSON(&r); err != nil {
		core.WriteResponse(ctx, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)
		return
	}
	if strings.TrimSpace(username) == "" {
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "用户名不能为空"), nil)
		return
	}
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		errMsg := strings.Join(errs, ":")
		log.Warnf("[control] 用户名校验失败: username=%s, error=%s", username, errMsg)
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "用户名不合法: %s", errMsg), nil)
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
	//查询现有用户
	existingUser, err := u.srv.Users().Get(c, username, metav1.GetOptions{}, u.options)
	if err != nil {
		log.Errorf("[control] 用户更新 service 层查询失败: username=%s, error=%v", username, err)
		core.WriteResponse(ctx, err, nil)
		return
	}
	//  安全防护：检查是否是防刷标记
	if existingUser.Name == sru.RATE_LIMIT_PREVENTION {
		err := errors.WithCode(code.ErrPasswordIncorrect, "用户名密码无效")
		core.WriteResponse(ctx, err, nil)
		return
	}

	// ✅ 只更新非nil的字段
	if r.Nickname != "" {
		existingUser.Nickname = r.Nickname
	}
	if r.Email != "" {
		existingUser.Email = r.Email
	}
	if r.Phone != "" {
		existingUser.Phone = r.Phone
	}
	// 4. ✅ 状态值验证（在Controller层进行输入验证）
	if r.Status != 0 { // 如果客户端提供了状态值
		if r.Status != 1 {
			// ❌ 非法状态值，直接返回错误
			core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "状态值必须为0或1"), nil)
			return
		}
		existingUser.Status = r.Status
	}

	// 5. ✅ 管理员状态验证
	if r.IsAdmin != 0 { // 如果客户端提供了管理员状态
		if r.IsAdmin != 1 {
			// ❌ 非法管理员状态，直接返回错误
			core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "管理员状态必须为0或1"), nil)
			return
		}
		existingUser.IsAdmin = r.IsAdmin
	}

	if errs := existingUser.ValidateUpdate(); len(errs) != 0 {
		core.WriteResponse(ctx, errors.WithCode(code.ErrValidation, "%s", errs.ToAggregate().Error()), nil)
		return
	}

	// 传入正确的对象（existingUser）
	if err := u.srv.Users().Update(c, existingUser,
		metav1.UpdateOptions{}, u.options); err != nil {
		log.Errorf("[control] 用户更新 service 层失败: username=%s, error=%v", username, err)
		core.WriteResponse(ctx, err, nil)
		return
	}

	// 构建成功数据
	successData := gin.H{
		"update_user":    existingUser,
		"operator":       common.GetUsername(ctx.Request.Context()),
		"operation_time": time.Now().Format(time.RFC3339),
		"operation_type": "create",
		"code":           code.ErrSuccess,
	}

	core.WriteResponse(ctx, nil, successData)
}
