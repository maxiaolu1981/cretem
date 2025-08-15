/*
包摘要
该包（auth）实现了一个自动认证策略（AutoStrategy），能够根据请求头中 Authorization 的前缀（Basic 或 Bearer）自动选择对应的认证方式（基础认证或 JWT 认证），实现了多种认证方式的统一入口和灵活切换。
核心流程
定义自动认证策略：AutoStrategy 结构体包含两种认证策略（basic 基础认证和 jwt JWT 认证）。
解析认证头：从请求的 Authorization 头中提取认证类型（前缀）和凭据，校验格式合法性。
选择认证策略：根据认证头前缀（Basic 或 Bearer）自动切换到对应的认证策略；若前缀不识别，则返回错误。
执行认证逻辑：通过认证操作器（operator）执行选中的认证策略，完成身份验证；认证失败则终止请求，成功则继续后续流程。
*/

// 包 auth 提供自动认证策略实现，支持根据 Authorization 头自动选择 Basic 或 Bearer (JWT) 认证
package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
)

// authHeaderCount 定义合法 Authorization 头的分割后长度（格式："类型 凭据"，如 "Bearer token"）
const authHeaderCount = 2

// AutoStrategy 自动认证策略，能根据 Authorization 头自动选择 Basic 或 Bearer (JWT) 认证
type AutoStrategy struct {
	basic middleware.AuthStrategy // 基础认证策略（对应 Basic 类型）
	jwt   middleware.AuthStrategy // JWT 认证策略（对应 Bearer 类型）
}

// 编译期断言：确保 AutoStrategy 实现了 middleware.AuthStrategy 接口
var _ middleware.AuthStrategy = &AutoStrategy{}

// NewAutoStrategy 创建自动认证策略实例
// 参数 basic：基础认证策略实例
// 参数 jwt：JWT 认证策略实例
// 返回值：初始化后的 AutoStrategy 对象
func NewAutoStrategy(basic, jwt middleware.AuthStrategy) AutoStrategy {
	return AutoStrategy{
		basic: basic,
		jwt:   jwt,
	}
}

// AuthFunc 实现 middleware.AuthStrategy 接口的 AuthFunc 方法，返回 Gin 中间件函数
// 该中间件会根据 Authorization 头自动选择对应的认证方式
func (a AutoStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 初始化认证操作器，用于动态设置和执行认证策略
		operator := middleware.AuthOperator{}
		// 分割 Authorization 头（格式："类型 凭据"，如 "Bearer eyJhbGciOiJIUzI1Ni..."）
		authHeader := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)

		// 校验 Authorization 头格式是否合法（必须包含类型和凭据两部分）
		if len(authHeader) != authHeaderCount {
			// 格式错误时返回错误响应，并终止请求
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrInvalidAuthHeader, "Authorization 头格式错误（正确格式：\"类型 凭据\"）。"),
				nil,
			)
			c.Abort() // 终止请求流转，不再执行后续中间件和业务逻辑
			return
		}

		// 根据认证类型前缀选择对应的认证策略
		switch authHeader[0] {
		case "Basic":
			// 若前缀为 Basic，使用基础认证策略
			operator.SetStrategy(a.basic)
		case "Bearer":
			// 若前缀为 Bearer，使用 JWT 认证策略
			operator.SetStrategy(a.jwt)
		default:
			// 不支持的认证类型，返回错误
			core.WriteResponse(
				c,
				errors.WithCode(code.ErrSignatureInvalid, "不支持的 Authorization 头类型。"),
				nil,
			)
			c.Abort()
			return
		}

		// 执行选中的认证策略（调用对应策略的 AuthFunc 方法）
		operator.AuthFunc()(c)

		// 认证通过后，继续执行后续中间件和业务逻辑
		c.Next()
	}
}
