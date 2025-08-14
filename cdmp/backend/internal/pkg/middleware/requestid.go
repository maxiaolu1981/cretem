/*
包摘要
该包（package middleware）提供了基于 Gin 框架的通用中间件，核心功能包括：
为每个请求生成 / 传递唯一的 X-Request-ID，用于全链路追踪。
提供带 RequestID 的日志格式化配置，方便关联请求日志。
封装从上下文或请求头中获取 RequestID 的工具方法。
核心流程
RequestID 中间件工作流程：
检查请求头中是否存在 X-Request-ID，若存在则复用。
若不存在，生成 UUID 作为 RequestID，并设置到请求头、上下文和响应头。
调用 c.Next() 传递请求给下一个中间件或业务处理器。
日志格式化流程：
通过 GetLoggerConfig 生成 Gin 日志配置，支持自定义输出目标和忽略路径。
默认日志格式（GetDefaultLogFormatterWithRequestID）包含响应状态码、客户端 IP、请求耗时、HTTP 方法、路径等信息，便于追踪单个请求的处理详情。


*/

package middleware

import (
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
)

// XRequestIDKey 定义请求ID在头信息中的键名
const (
	XRequestIDKey = "X-Request-ID"
)

// RequestID 是一个中间件，用于为每个请求注入唯一的 'X-Request-ID'，
// 并将其存储在上下文、请求头和响应头中，便于全链路追踪。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查请求头中是否已有 X-Request-ID（可能由上游服务传递）
		rid := c.GetHeader(XRequestIDKey)

		// 若请求头中没有，则生成新的 UUID v4 作为 RequestID
		if rid == "" {
			rid = uuid.Must(uuid.NewV4()).String()   // 生成 UUID 并转为字符串
			c.Request.Header.Set(XRequestIDKey, rid) // 设置到请求头
			c.Set(XRequestIDKey, rid)                // 存入上下文，供后续处理器使用
		}

		// 将 RequestID 设置到响应头，返回给客户端
		c.Writer.Header().Set(XRequestIDKey, rid)
		// 调用下一个中间件或业务处理器
		c.Next()
	}
}

// GetLoggerConfig 返回 Gin 日志配置，支持自定义日志格式化器、输出目标和忽略路径。
// 默认输出到标准输出（gin.DefaultWriter = os.Stdout）。
// 参考：https://github.com/gin-gonic/gin#custom-log-format
func GetLoggerConfig(formatter gin.LogFormatter, output io.Writer, skipPaths []string) gin.LoggerConfig {
	// 若未指定格式化器，使用带 RequestID 的默认格式
	if formatter == nil {
		formatter = GetDefaultLogFormatterWithRequestID()
	}

	return gin.LoggerConfig{
		Formatter: formatter, // 日志格式化器
		Output:    output,    // 日志输出目标（如文件、标准输出）
		SkipPaths: skipPaths, // 需要忽略日志的路径（如健康检查接口）
	}
}

// GetDefaultLogFormatterWithRequestID 返回包含 'RequestID' 的默认日志格式化器。
// 日志内容包括：响应状态码、客户端IP、请求耗时、HTTP方法、路径、错误信息等。
func GetDefaultLogFormatterWithRequestID() gin.LogFormatter {
	return func(param gin.LogFormatterParams) string {
		var statusColor, methodColor, resetColor string
		// 若输出支持彩色，则为状态码和方法名添加颜色
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor() // 状态码颜色（如成功200为绿色，错误500为红色）
			methodColor = param.MethodColor()     // HTTP方法颜色（如GET为蓝色，POST为黄色）
			resetColor = param.ResetColor()       // 重置颜色
		}

		// 若请求耗时超过1分钟，截断为秒级（避免显示过多小数）
		if param.Latency > time.Minute {
			param.Latency -= param.Latency % time.Second
		}

		// 格式化日志字符串
		return fmt.Sprintf("%s%3d%s - [%s] \"%v %s%s%s %s\" %s",
			statusColor, param.StatusCode, resetColor, // 带颜色的状态码
			param.ClientIP,                        // 客户端IP
			param.Latency,                         // 请求处理耗时
			methodColor, param.Method, resetColor, // 带颜色的HTTP方法
			param.Path,         // 请求路径
			param.ErrorMessage, // 错误信息（若有）
		)
	}
}

// GetRequestIDFromContext 从 Gin 上下文中获取 'RequestID'（若存在）。
// 需配合 RequestID 中间件使用，返回空字符串表示未找到。
func GetRequestIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(XRequestIDKey); ok {
		if requestID, ok := v.(string); ok {
			return requestID
		}
	}

	return ""
}

// GetRequestIDFromHeaders 从请求头中获取 'RequestID'（若存在）。
// 适用于需要直接从头部读取的场景，返回空字符串表示未找到。
func GetRequestIDFromHeaders(c *gin.Context) string {
	return c.Request.Header.Get(XRequestIDKey)
}
