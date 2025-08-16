/*
1. user 包聚焦用户领域的 API 控制器逻辑，通过 UserController 实现用户资源的 HTTP 接口（如 List 方法）。核心职责：
---HTTP 协议适配：解析 HTTP 请求参数（如分页、过滤条件），转换为业务层可识别的格式；
---权限与日志：限定接口权限（仅管理员可调用）、记录操作日志；
----错误处理：通过统一错误码（code.ErrBind 等）封装错误，标准化 API 响应。
二、核心流程
代码围绕 “用户列表查询” 的 HTTP 接口设计，关键流程拆解：
1. 请求解析（参数绑定）
通过 c.ShouldBindQuery(&r) 解析 URL 查询参数（如分页 offset、limit），转换为 metav1.ListOptions 结构体；
若解析失败（如参数格式错误），返回标准化错误（code.ErrBind）。
2. 业务层调用
调用 u.srv.Users().List(c, r)，将解析后的参数传递给业务层（Service），执行用户列表查询；
业务层返回 users 或 err，控制器需处理两种分支。
3. 响应封装
若业务层返回错误（如数据库查询失败），通过 core.WriteResponse 封装错误码 + 错误信息，返回给前端；
若查询成功，通过 core.WriteResponse 返回用户列表数据（users），确保响应格式标准化。
*/
package user

import (
	"github.com/gin-gonic/gin" // Gin 框架：用于构建 HTTP 接口

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"                   // 错误码定义（如参数绑定失败）
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"                    // 日志工具
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"           // 响应封装工具
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1" // 元数据规范（分页/过滤参数）
	"github.com/maxiaolu1981/cretem/nexuscore/errors"                        // 错误封装工具
)

// List 是用户列表查询的 HTTP 接口，仅管理员可调用。
// 职责：
// 1. 解析 HTTP 查询参数（分页、过滤条件）；
// 2. 调用业务层（srv）执行用户列表查询；
// 3. 封装错误或成功响应，返回给前端。
func (u *UserController) List(c *gin.Context) {
	// 记录操作日志：标记“用户列表查询”接口被调用
	log.L(c).Info("list user function called.")

	// 定义请求参数结构体（遵循 metav1.ListOptions 规范）
	var r metav1.ListOptions
	// 解析 URL 查询参数（如 ?offset=0&limit=10）到 r
	if err := c.ShouldBindQuery(&r); err != nil {
		// 参数绑定失败：封装错误码（code.ErrBind），返回给前端
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "%s", err.Error()), nil)
		return
	}

	// 调用业务层（srv）查询用户列表
	users, err := u.srv.Users().List(c, r)
	if err != nil {
		// 业务逻辑失败：直接返回业务层错误（已封装错误码）
		core.WriteResponse(c, err, nil)
		return
	}

	// 查询成功：返回用户列表数据
	core.WriteResponse(c, nil, users)
}
