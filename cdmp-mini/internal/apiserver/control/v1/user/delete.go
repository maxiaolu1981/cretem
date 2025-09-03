package user

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) Delete(ctx *gin.Context) {
	logger := log.L(ctx).WithValues(
		"controller", "UserController",
		"action", "Delete",
		"client_ip", ctx.ClientIP(), // 客户端IP
		"method", ctx.Request.Method, // 请求方法
		"path", ctx.FullPath(), // 请求路径 操作的资源ID
		"user_agent", ctx.Request.UserAgent(), // 用户代理
		"Unscoped", true,
	)
	stdCtx := ctx.Request.Context()
	operator := common.GetUsername(stdCtx)
	if operator == "" {
		logger.Warn("当前无法获取操作者的用户名,可能未登录")
		core.WriteResponse(ctx, errors.WithCode(code.ErrUnauthorized, "要删除的用户名为空"), nil)
		return
	}
	logger = logger.WithValues("operator", operator)
	logger.Infof("开始处理用户:[%s]删除请求", operator)
	deleteUsername := ctx.Param("name")
	if deleteUsername == "" {
		logger.Error("要删除的用户名为空")
		core.WriteResponse(ctx, errors.WithCode(code.ErrUserNotFound, "要删除的用户名为空"), nil)
		return
	}
	logger = logger.WithValues("delete_username", deleteUsername)
	logger.Info("开始处理用户删除")
	_, err := store.Client().Users().Get(stdCtx, deleteUsername, metav1.GetOptions{})
	if err != nil {
		// 关键：匹配 Get 方法返回的 "用户不存在" 错误码 code.ErrUserNotFound
		if errors.IsCode(err, code.ErrUserNotFound) {
			logger.Info("用户不存在，无需删除")
			core.WriteResponse(ctx, nil, "用户不存在，无需删除")
			return
		}
		// 其他错误（如超时、数据库异常）
		logger.Errorf("查询用户信息失败%s", err)
		core.WriteResponse(ctx, err, nil) // 直接返回 Get 方法处理后的错误（已包含错误码）
		return
	}

	if err := u.srv.Users().Delete(stdCtx, deleteUsername, metav1.DeleteOptions{Unscoped: false}); err != nil {
		logger.Errorf("用户删除失败%v", err)
		core.WriteResponse(ctx, err, nil)
		return
	}
	logger.Info("用户删除成功")
	core.WriteResponse(ctx, nil, nil)
}

func (u *UserController) ForceDelete(ctx *gin.Context) {
	logger := log.L(ctx).WithValues(
		"controller", "UserController",
		"action", "Delete",
		"client_ip", ctx.ClientIP(), // 客户端IP
		"method", ctx.Request.Method, // 请求方法
		"path", ctx.FullPath(), // 请求路径 操作的资源ID
		"user_agent", ctx.Request.UserAgent(), // 用户代理
		"Unscoped", true,
	)
	stdCtx := ctx.Request.Context()
	operator := common.GetUsername(stdCtx)
	if operator == "" {
		logger.Warn("当前无法获取操作者的用户名,可能未登录")
		core.WriteResponse(ctx, errors.WithCode(code.ErrUnauthorized, "要删除的用户名为空"), nil)
		return
	}
	logger = logger.WithValues("operator", operator)
	logger.Infof("开始处理用户:[%s]删除请求", operator)
	deleteUsername := ctx.Param("name")
	if deleteUsername == "" {
		logger.Error("要删除的用户名为空")
		core.WriteResponse(ctx, errors.WithCode(code.ErrUserNotFound, "要删除的用户名为空"), nil)
		return
	}
	logger = logger.WithValues("delete_username", deleteUsername)
	logger.Info("开始处理用户删除")
	_, err := store.Client().Users().Get(stdCtx, deleteUsername, metav1.GetOptions{})
	if err != nil {
		// 关键：匹配 Get 方法返回的 "用户不存在" 错误码 code.ErrUserNotFound
		if errors.IsCode(err, code.ErrUserNotFound) {
			logger.Info("用户不存在，无需删除")
			core.WriteResponse(ctx, nil, "用户不存在，无需删除")
			return
		}
		// 其他错误（如超时、数据库异常）
		logger.Errorf("查询用户信息失败%s", err)
		core.WriteResponse(ctx, err, nil) // 直接返回 Get 方法处理后的错误（已包含错误码）
		return
	}

	if err := u.srv.Users().Delete(stdCtx, deleteUsername, metav1.DeleteOptions{Unscoped: true}); err != nil {
		logger.Errorf("用户删除失败%v", err)
		core.WriteResponse(ctx, err, nil)
		return
	}
	logger.Info("用户删除成功")
	core.WriteResponse(ctx, nil, nil)
}
