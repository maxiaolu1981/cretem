/*
该包（auth）提供了基于 JWT（JSON Web Token）的认证策略实现，通过封装第三方库 gin-jwt 的中间件功能，使其适配项目内部统一的认证接口（middleware.AuthStrategy），用于在 Gin 框架中实现 JWT 令牌的生成、验证及权限控制等认证逻辑。
核心流程
定义 JWT 认证策略结构：通过 JWTStrategy 结构体嵌入 ginjwt.GinJWTMiddleware，继承第三方 JWT 中间件的功能。
实现认证接口：JWTStrategy 实现 middleware.AuthStrategy 接口的 AuthFunc 方法，将 JWT 中间件的核心逻辑（MiddlewareFunc）暴露为 Gin 可识别的中间件。
构造函数初始化：NewJWTStrategy 函数接收配置好的 ginjwt.GinJWTMiddleware 实例，创建 JWTStrategy 对象，完成第三方中间件到自定义策略的封装。
路由中使用：在 Gin 路由中通过 AuthFunc() 方法调用 JWT 认证中间件，对请求进行令牌验证和权限校验。
*/
package auth

import (
	ginjwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
)

// AuthzAudience defines the value of jwt audience field.
const AuthzAudience = "iam.authz.marmotedu.com"

// JWTStrategy defines jwt bearer authentication strategy.
type JWTStrategy struct {
	ginjwt.GinJWTMiddleware
}

var _ middleware.AuthStrategy = &JWTStrategy{}

// NewJWTStrategy create jwt bearer strategy with GinJWTMiddleware.
func NewJWTStrategy(gjwt ginjwt.GinJWTMiddleware) JWTStrategy {
	return JWTStrategy{gjwt}
}

// AuthFunc defines jwt bearer strategy as the gin authentication middleware.
func (j JWTStrategy) AuthFunc() gin.HandlerFunc {
	return j.MiddlewareFunc()
}
