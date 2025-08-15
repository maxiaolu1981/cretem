// 包 middleware 提供了认证策略的抽象与切换机制。
// 主要流程：
// 1. 定义 AuthStrategy 接口，抽象不同认证方式（如 JWT、Basic 等）。
// 2. 通过 AuthOperator 结构体动态切换认证策略。
// 3. 业务中调用 AuthOperator.AuthFunc()，自动执行当前策略的认证逻辑。
//
// 用法示例：
//
//	operator := &AuthOperator{}
//	operator.SetStrategy(jwtStrategy)
//	router.Use(operator.AuthFunc())
//
// 支持扩展：可自定义实现 AuthStrategy 接口，支持多种认证方式。
package middleware

import (
	"github.com/gin-gonic/gin"
)

// AuthStrategy 认证策略接口，定义资源认证方法。
// 实现该接口可支持多种认证方式（如 JWT、Basic 等）。
type AuthStrategy interface {
	AuthFunc() gin.HandlerFunc // 返回 gin 的认证中间件
}

// AuthOperator 用于在多种认证策略间切换。
type AuthOperator struct {
	strategy AuthStrategy // 当前使用的认证策略
}

// SetStrategy 设置新的认证策略。
func (operator *AuthOperator) SetStrategy(strategy AuthStrategy) {
	operator.strategy = strategy
}

// AuthFunc 执行当前认证策略的认证方法。
func (operator *AuthOperator) AuthFunc() gin.HandlerFunc {
	return operator.strategy.AuthFunc()
}
