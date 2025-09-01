/*
这个包是IAM项目的Basic认证策略实现模块，提供了基于用户名和密码的基础认证功能。以下是包摘要：

包功能
Basic认证: 实现HTTP Basic认证标准

凭证验证: 通过回调函数验证用户名和密码

中间件集成: 提供Gin中间件认证接口

核心结构体
BasicStrategy
Basic认证策略结构体：

go
type BasicStrategy struct {
    compare func(username string, password string) bool // 凭证验证回调函数
}
主要方法
NewBasicStrategy(compare func()) BasicStrategy
创建Basic认证策略实例，需要传入验证函数

AuthFunc() gin.HandlerFunc
生成Gin认证中间件函数

认证流程
解析头部: 检查Authorization头部格式是否为"Basic"

解码凭证: Base64解码认证凭证

分割凭证: 分割用户名和密码（以冒号分隔）

验证凭证: 调用compare函数验证用户名密码

设置上下文: 认证成功后设置用户名到Gin上下文

错误处理: 任何步骤失败都返回认证错误

技术细节
编码格式: 使用Base64标准编码解码

分隔符: 用户名密码以冒号:分隔

错误类型: 统一返回ErrSignatureInvalid错误

上下文键: 使用middleware.UsernameKey存储用户名

设计特点
依赖注入: 通过回调函数注入验证逻辑，与具体存储解耦

标准兼容: 完全遵循HTTP Basic认证标准

灵活验证: compare函数可以连接数据库、LDAP或其他认证源

中间件友好: 完全兼容Gin中间件模式

安全考虑
使用Base64编码（非加密），建议配合HTTPS使用

验证函数应实现密码安全比较（防时序攻击）

使用场景
内部系统API认证

简单的用户名密码认证需求

与其他认证策略组合使用（如AutoStrategy）

这个包提供了简单而标准的Basic认证实现，为IAM项目提供了基础的认证能力。


*/

package auth

import (
	"encoding/base64"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

type BasicStrategy struct {
	compare func(username string, password string) bool
}

func NewBasicStrategy(compare func(username string, password string) bool) *BasicStrategy {
	return &BasicStrategy{compare: compare}
}

func (b *BasicStrategy) AuthFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
		if len(auth) != 2 || auth[0] != "Basic" {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidAuthHeader, "无效的header"), nil)
			c.Abort()
			return
		}
		payload, _ := base64.StdEncoding.DecodeString(auth[1])
		pair := strings.SplitN(string(payload), ":", 2)
		if len(pair) != 2 {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidAuthHeader, "无效的头"), nil)
			c.Abort()
			return
		}
		if b.compare(pair[0], pair[1]) {
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidAuthHeader, "无效的头"), nil)
			c.Abort()
			return
		}
		c.Set(common.UsernameKey, pair[0])
		c.Next()

	}
}
