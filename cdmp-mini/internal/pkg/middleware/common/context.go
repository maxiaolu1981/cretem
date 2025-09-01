/*
这个包是IAM项目的上下文中间件模块，负责在Gin上下文中注入通用字段。以下是包摘要：

包功能
上下文增强: 向Gin上下文注入通用字段

日志集成: 设置日志相关的上下文信息

统一字段管理: 提供统一的上下文键常量

核心常量
UsernameKey
go
const UsernameKey = "username" // 代表资源所有者的上下文键
用于在Gin上下文中存储和获取用户名信息

中间件函数
Context() gin.HandlerFunc
创建上下文注入中间件，功能包括：

设置请求ID: 将XRequestIDKey的值设置到log.KeyRequestID

设置用户名: 将UsernameKey的值设置到log.KeyUsername

继续执行: 调用c.Next()继续处理链

设计特点
日志集成: 为结构化日志提供统一的上下文字段

信息传递: 在中间件链中传递用户身份和请求追踪信息

常量管理: 集中管理上下文键名，避免硬编码

轻量级: 简单的键值设置操作，性能开销小

使用场景
请求追踪: 为每个请求注入唯一的请求ID

用户识别: 在上下文中传递认证后的用户身份信息

日志记录: 为后续的日志中间件提供结构化数据

依赖关系
日志系统: 依赖pkg/log包定义的日志键常量

请求追踪: 假设前面中间件已设置XRequestIDKey

这个中间件作为信息传递的桥梁，确保关键的上下文信息（如用户身份、请求ID）在整个请求处理生命周期中可用。
*/
package common

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

// 定义上下文键类型（避免字符串键冲突）
type contextKey string

const (
	KeyRequestID contextKey = "request_id"
	KeyUsername  contextKey = "username"
)

const (
	XRequestIDKey = "X-Request-ID"
	UsernameKey   = "username"
)

// Context 中间件 - 增强版
func Context() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 设置Gin上下文的值（保持向后兼容）
		requestID := c.GetString(XRequestIDKey)
		username := c.GetString(UsernameKey)

		c.Set(log.KeyRequestID, requestID)
		c.Set(log.KeyUsername, username)

		// 2. 增强：在标准context中设置值（用于Service层）
		ctx := c.Request.Context()
		if requestID != "" {
			ctx = context.WithValue(ctx, KeyRequestID, requestID)
		}
		if username != "" {
			ctx = context.WithValue(ctx, KeyUsername, username)
		}

		// 3. 更新请求的上下文
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// 辅助函数：从标准context中获取值
func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(KeyRequestID).(string); ok {
		return val
	}
	return "unknown"
}

func GetUsername(ctx context.Context) string {
	if val, ok := ctx.Value(KeyUsername).(string); ok {
		return val
	}
	return "unknown"
}
