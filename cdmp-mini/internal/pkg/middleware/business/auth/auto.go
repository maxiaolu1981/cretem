/*
这个包是IAM项目的认证策略模块，实现了自动选择认证方式的智能策略。以下是包摘要：

包功能
自动认证: 根据Authorization头部自动选择Basic或Bearer认证

策略路由: 将认证请求路由到相应的具体认证策略

统一接口: 提供统一的Gin中间件认证接口

核心结构体
AutoStrategy
自动认证策略结构体：

go

	type AutoStrategy struct {
	    basic middleware.AuthStrategy  // Basic认证策略
	    jwt   middleware.AuthStrategy  // JWT Bearer认证策略
	}

主要方法
NewAutoStrategy(basic, jwt middleware.AuthStrategy) AutoStrategy
创建自动认证策略实例

AuthFunc() gin.HandlerFunc
生成Gin认证中间件函数，实现：

解析头部: 解析Authorization头部信息

策略选择: 根据"Basic"或"Bearer"选择对应策略

错误处理: 处理无效的认证头部格式

认证执行: 调用具体策略的认证方法

认证流程
检查Authorization头部是否存在且格式正确

根据认证类型选择策略：

# Basic → Basic认证策略

# Bearer → JWT认证策略

执行选择的认证策略

如果认证失败，返回相应的错误响应

错误处理
ErrInvalidAuthHeader: Authorization头部格式错误

ErrSignatureInvalid: 无法识别的认证类型

统一使用core.WriteResponse返回错误信息

设计特点
策略模式: 使用策略模式实现认证算法的灵活切换

装饰器模式: 包装具体认证策略，提供统一接口

开闭原则: 易于扩展新的认证策略类型

中间件兼容: 完全兼容Gin中间件接口标准

使用场景
API网关的统一认证入口

支持多种认证方式的微服务

需要向后兼容不同认证协议的系统

这个包作为认证系统的智能路由层，为IAM项目提供了灵活、可扩展的多认证策略支持。
*/
package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
	middleware "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

const authHeaderCount = 2

type AutoStrategy struct {
	basicStrategy middleware.AuthStrategy
	jwtStrategy   middleware.AuthStrategy
}

func NewAutoStrategy(basic, jwt middleware.AuthStrategy) AutoStrategy {
	return AutoStrategy{
		basicStrategy: basic,
		jwtStrategy:   jwt,
	}
}

func (a *AutoStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		operator := middleware.AuthOperator{}
		auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
		if len(auth) != authHeaderCount {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidAuthHeader, "无效的header"), nil)
			c.Abort()
			return
		}
		switch auth[0] {
		case "Basic":
			operator.SetAuthStrategy(a.basicStrategy)
		case "Bearer":
			operator.SetAuthStrategy(a.jwtStrategy)
		}
		operator.AuthFunc()(c)
		c.Next()
	}
}
