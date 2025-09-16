package user

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
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
	_, err := interfaces.Client().Users().Get(stdCtx, deleteUsername, metav1.GetOptions{})
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

	if err := u.srv.Users().Delete(stdCtx, deleteUsername, false, metav1.DeleteOptions{Unscoped: false}); err != nil {
		logger.Errorf("用户删除失败%v", err)
		core.WriteResponse(ctx, err, nil)
		return
	}
	logger.Info("用户删除成功")
	core.WriteResponse(ctx, nil, nil)
}

// ForceDelete 强制删除用户接口（RESTful 路径：DELETE /v1/users/:name/force）
// @Summary 强制删除指定用户
// @Description 忽略用户关联资源，直接删除用户（需管理员权限）
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param name path string true "用户名"
// @Header 200 {string} X-Request-ID "请求追踪ID"
// @Success 204 "删除成功（无响应体）"
// @Failure 400 {object} core.ErrorResponse "参数错误"
// @Failure 401 {object} core.ErrorResponse "未登录"
// @Failure 404 {object} core.ErrorResponse "用户不存在"
// @Failure 500 {object} core.ErrorResponse "服务内部错误"
// @Router /v1/users/{name}/force [delete]
func (u *UserController) ForceDelete(ctx *gin.Context) {
	// 1. 初始化日志：提前占位关键信息（避免前半段日志缺失上下文）
	logger := log.L(ctx).WithValues(
		"module", "user_controller",
		"action", "ForceDelete",
		"client_ip", ctx.ClientIP(),
		"http_method", ctx.Request.Method,
		"request_path", ctx.FullPath(),
		"user_agent", ctx.Request.UserAgent(),
		"unscoped", true, // 强制删除标记
		"operator", "unknown", // 操作者（后续补全）
		"delete_username", "unknown", // 待删除用户名（后续补全）
	)

	// 2. 从上下文提取标准库 context（传递认证、日志等元数据）
	stdCtx := ctx.Request.Context()

	// 3. 校验：获取操作者（未登录场景）
	operator := common.GetUsername(stdCtx)
	if operator == "" {
		logger.Warn("用户没有登录,无法进行删除")
		// 用 errors.WithCode 创建“未登录”错误（业务码→HTTP 401）
		err := errors.WithCode(
			code.ErrUnauthorized, // 业务码：1001（未授权）
			"未登录，无法执行用户强制删除操作",
		)
		core.WriteResponse(ctx, err, nil)
		return
	}
	// 补全操作者日志（后续所有日志均包含操作者）
	logger = logger.WithValues("operator", operator)

	// 4. 校验：提取并检查待删除用户名（参数无效场景）
	deleteUsername := ctx.Param("name")
	if strings.TrimSpace(deleteUsername) == "" {
		logger.Warnf("用户名为空,无法删除")
		// 用 errors.WithCode 创建“参数无效”错误（业务码→HTTP 400）
		err := errors.WithCode(
			code.ErrInvalidParameter, // 业务码：1002（参数错误）
			"参数错误：待删除的用户名为空，请检查请求路径（如 /v1/users/{name}/force）",
		)
		core.WriteResponse(ctx, err, nil)
		return
	}

	if errs := validation.IsQualifiedName(deleteUsername); len(errs) > 0 {
		errsMsg := strings.Join(errs, ":")
		log.Warnw("用户名不合法:", errsMsg)
		core.WriteResponse(ctx, errors.WithCode(code.ErrInvalidParameter, "用户名不合法:%s", errsMsg), nil)
		return
	}
	// 补全待删除用户名日志
	logger = logger.WithValues("delete_username", deleteUsername)

	// 5. 业务校验：查询用户是否存在（资源不存在/服务端错误场景）
	_, rawGetErr := interfaces.Client().Users().Get(stdCtx, deleteUsername, metav1.GetOptions{})
	if rawGetErr != nil {
		// 子场景1：用户不存在（业务码→HTTP 404）
		if errors.IsCode(rawGetErr, code.ErrUserNotFound) {
			logger.Warnf("用户 [%s]不存在,无法删除", deleteUsername)
			// 用 errors.WrapC 包装原始错误（保留错误链），绑定“用户不存在”业务码
			err := errors.WrapC(
				rawGetErr,            // 原始查询错误（含堆栈，便于调试）
				code.ErrUserNotFound, // 业务码：1003（用户不存在）
				"用户[%s]不存在，无法执行强制删除操作",
				deleteUsername,
			)
			core.WriteResponse(ctx, err, nil)
			return
		}

		// 子场景2：查询失败（服务端错误，业务码→HTTP 500）
		logger.Warnf("数据库错误 [%s]: %+v", deleteUsername, rawGetErr)
		// 用 errors.WrapC 包装原始错误，绑定“服务端内部错误”业务码
		err := errors.WrapC(
			rawGetErr,              // 原始错误（如 DB 超时、权限不足）
			code.ErrInternalServer, // 业务码：5001（服务端错误）
			"查询用户[%s]信息失败，请稍后重试",
			deleteUsername,
		)
		core.WriteResponse(ctx, err, nil)
		return
	}
	logger.Infof("用户 [%s] 存在, 开始进行删除", deleteUsername)

	// 6. 核心业务：调用服务层执行强制删除（删除失败场景）
	logger.Debugf("控制层:调用服务层方法,开始删除用户[%s]", deleteUsername)
	rawDelErr := u.srv.Users().Delete(
		stdCtx,
		deleteUsername,
		true,                                 // force=true：强制删除
		metav1.DeleteOptions{Unscoped: true}, // Unscoped：忽略命名空间限制
	)
	if rawDelErr != nil {
		logger.Errorf("失败删除用户:[%s]: %+v", deleteUsername, rawDelErr)
		// 用 errors.WrapC 包装删除错误，绑定“服务端错误”业务码
		err := errors.WrapC(
			rawDelErr,              // 原始删除错误（如 DB 事务失败、锁冲突）
			code.ErrInternalServer, // 业务码→HTTP 500
			"用户[%s]强制删除失败，请稍后重试",
			deleteUsername,
		)
		core.WriteResponse(ctx, err, nil)
		return
	}

	// 7. 成功场景：返回 RESTful 标准 204 No Content（无响应体）
	logger.Infof("用户[%s]已经删除", deleteUsername)
	core.WriteDeleteSuccess(ctx)
}
