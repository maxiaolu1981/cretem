// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// 建议：用常量定义路由路径，避免硬编码（提升扩展性）
const (
	RouteUsersCreate         = "/v1/users"                       // 用户创建
	RouteUsersDetail         = "/v1/users/:name"                 // 用户详情（查/改/删）
	RouteUsersChangePassword = "/v1/users/:name/change_password" // 修改密码
)

// Validation 权限校验中间件：确保用户有对应资源的操作权限
func Validation() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 第一步：先执行 isAdmin，区分错误类型（数据库错误 vs 非管理员）
		err := isAdmin(c)
		if err != nil {
			// 关键：判断错误类型，优先处理服务端错误（数据库错误）
			if errors.IsCode(err, code.ErrDatabase) {
				// 场景：查询用户信息失败（服务端错误）→ 返回 500
				core.WriteResponse(c,
					errors.WithCode(code.ErrDatabase, "权限校验失败：查询用户信息异常，请稍后重试"),
					nil,
				)
				c.Abort()
				return
			}

			// 第二步：仅处理「非管理员」错误（403），按路径判断权限
			fullPath := c.FullPath()
			switch fullPath {
			// case RouteUsersCreate:
			// 	// 非管理员仅允许 POST（创建用户），其他方法（GET/DELETE 等）返回 403
			// 	if c.Request.Method != http.MethodPost {
			// 		core.WriteResponse(c,
			// 			errors.WithCode(code.ErrPermissionDenied, "非管理员仅允许创建用户，不允许执行其他操作"),
			// 			nil,
			// 		)
			// 		c.Abort()
			// 		return
			// 	}

			case RouteUsersDetail, RouteUsersChangePassword:
				// 1. 正确获取用户名（用统一的 common.UsernameKey，避免硬编码）
				username, exists := c.Get(common.UsernameKey)
				if !exists || username == "" {
					core.WriteResponse(c,
						errors.WithCode(code.ErrUnauthorized, "权限校验失败：未获取到当前用户名"),
						nil,
					)
					c.Abort()
					return
				}
				currentUser := username.(string)
				targetUser := c.Param("name")

				// 2. 权限规则：
				// - 任何用户都不允许删除自己（DELETE 方法）
				// - 非管理员仅允许操作自己的账号（其他账号返回 403）
				if c.Request.Method == http.MethodDelete {
					core.WriteResponse(c,
						errors.WithCode(code.ErrPermissionDenied, "不允许删除自己的账号（需管理员操作）"),
						nil,
					)
					c.Abort()
					return
				}
				if currentUser != targetUser {
					core.WriteResponse(c,
						errors.WithCode(code.ErrPermissionDenied, "非管理员仅允许操作自己的账号，不允许修改他人信息"),
						nil,
					)
					c.Abort()
					return
				}

			default:
				// 其他未定义路径：非管理员直接返回 403（避免默认放行风险）
				core.WriteResponse(c,
					errors.WithCode(code.ErrPermissionDenied, "非管理员无权限访问此接口"),
					nil,
				)
				c.Abort()
				return
			}
		}

		// 管理员：放行所有操作
		c.Next()
	}
}

// isAdmin 判断当前用户是否为管理员（纯判断逻辑，不处理错误返回）
func isAdmin(c *gin.Context) error {
	// 1. 正确获取用户名（用 common.UsernameKey，与认证中间件统一）
	usernameVal, exists := c.Get(common.UsernameKey)
	if !exists || usernameVal == "" {
		return errors.WithCode(code.ErrUnauthorized, "判断管理员失败：未获取到当前用户名")
	}
	username := usernameVal.(string)

	// 2. 查询用户信息（处理数据库错误，过滤敏感信息）
	user, err := store.Client().Users().Get(c, username, metav1.GetOptions{})
	if err != nil {
		// 修复：不返回原始错误（避免敏感信息），返回用户友好提示
		return errors.WithCode(code.ErrDatabase, "查询用户信息失败")
	}

	// 3. 判断是否为管理员（IsAdmin == 1）
	if user.IsAdmin != 1 {
		return errors.WithCode(code.ErrPermissionDenied, "当前用户（%s）非管理员，无管理员权限", username)
	}

	return nil
}
