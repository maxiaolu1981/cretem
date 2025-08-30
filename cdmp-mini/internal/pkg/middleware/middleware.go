/*
这个包是IAM项目的中间件管理模块，提供了常用的HTTP中间件集合和统一注册功能。以下是包摘要：

包功能
中间件集合: 提供一系列常用的HTTP中间件

统一管理: 集中注册和管理所有中间件

安全增强: 添加安全相关的HTTP头

开发支持: 提供调试和日志中间件

核心变量
Middlewares
go
var Middlewares = defaultMiddlewares() // 已注册的中间件映射
存储所有默认中间件的名称和处理函数映射

中间件函数
NoCache
禁用客户端缓存中间件：

设置Cache-Control、Expires、Last-Modified头部

防止客户端缓存HTTP响应

Options
处理OPTIONS请求中间件：

设置CORS相关头部（允许的源、方法、头部）

直接返回200状态码并终止请求处理

Secure
安全增强中间件：

设置X-Frame-Options、X-Content-Type-Options、X-XSS-Protection

启用HSTS（HTTP严格传输安全）

默认中间件集合
defaultMiddlewares()
返回默认中间件映射，包含：

recovery: Gin恢复中间件（panic恢复）

secure: 安全头部中间件

options: OPTIONS请求处理中间件

nocache: 缓存禁用中间件

cors: CORS跨域中间件

requestid: 请求ID追踪中间件

logger: 请求日志中间件

dump: 请求响应调试中间件（gin-dump）

设计特点
开箱即用: 提供生产环境所需的完整中间件集合

安全优先: 包含多项安全增强功能

开发友好: 集成调试和日志中间件便于开发

模块化: 每个中间件功能单一，易于组合使用

使用场景
API服务器的中间件栈配置

统一的安全策略实施

请求生命周期管理和监控

这个包作为中间件基础设施，为IAM项目提供了完整、安全、可观测的HTTP处理中间件集合。
*/
package middleware

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	gindump "github.com/tpkeeper/gin-dump"
)

const (
	HeaderCORSOrigin              = "Access-Control-Allow-Origin"
	HeaderFrameOptions            = "X-Frame-Options"
	HeaderContentTypeOptions      = "X-Content-Type-Options"
	HeaderXSSProtection           = "X-XSS-Protection"
	HeaderStrictTransportSecurity = "Strict-Transport-Security"
	HeaderContentSecurityPolicy   = "Content-Security-Policy"
)

func Secure(c *gin.Context) {
	c.Header(HeaderCORSOrigin, "*")
	c.Header(HeaderFrameOptions, "DENY")
	c.Header(HeaderContentTypeOptions, "nosniff")
	c.Header(HeaderXSSProtection, "1; mode=block")
	c.Header(HeaderContentSecurityPolicy, "script-src 'self'")
	if c.Request.TLS != nil {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}
}

func NoCache(c *gin.Context) {
	// 更明确和现代的设置
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache") // 为了兼容HTTP/1.0
	c.Header("Expires", "0")
	c.Next()
}

// 基础中间件（所有环境共用）
var baseMiddlewares = map[string]gin.HandlerFunc{
	"recovery":  gin.Recovery(),
	"requestid": RequestID(),
	"secure":    Secure,
	"nocache":   NoCache,
}

// 开发环境特定中间件
var devMiddlewares = map[string]gin.HandlerFunc{
	"logger": devLogger(),    // 开发环境日志
	"cors":   DevCors(),      // 宽松的CORS配置
	"dump":   gindump.Dump(), // 请求响应dump
}

// 测试环境特定中间件
var testMiddlewares = map[string]gin.HandlerFunc{
	"logger": testLogger(), // 测试环境日志
	"cors":   TestCors(),   // 测试环境CORS
}

// 生产环境特定中间件
var prodMiddlewares = map[string]gin.HandlerFunc{
	"logger": productionLogger(), // 生产环境日志
	"cors":   ProductionCors(),   // 生产环境CORS
}

// devLogger 开发环境日志（彩色输出）
func devLogger() gin.HandlerFunc {
	return LoggerWithConfig(gin.LoggerConfig{
		Formatter: defaultLogFormatter,
		Output:    gin.DefaultWriter,
		SkipPaths: []string{"/healthz"},
	})
}

// testLogger 测试环境日志（简洁格式）
func testLogger() gin.HandlerFunc {
	return LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("TEST %s %s %s - %d (%s)\n",
				param.TimeStamp.Format("15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
				param.Latency)
		},
		Output:    os.Stdout,
		SkipPaths: []string{"/healthz", "/metrics"},
	})
}

// productionLogger 生产环境日志（JSON格式）
func productionLogger() gin.HandlerFunc {
	// 创建日志文件
	file, err := os.OpenFile("/var/log/app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		file = os.Stdout // 失败时回退到标准输出
	}

	return LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			logEntry := map[string]interface{}{
				"timestamp": param.TimeStamp.Format(time.RFC3339),
				"ip":        param.ClientIP,
				"method":    param.Method,
				"path":      param.Path,
				"status":    param.StatusCode,
				"latency":   param.Latency.Milliseconds(),
				"error":     param.ErrorMessage,
			}
			jsonData, _ := json.Marshal(logEntry)
			return string(jsonData) + "\n"
		},
		Output:    file,
		SkipPaths: []string{"/healthz", "/metrics", "/readyz"},
	})
}

// getEnv 获取当前环境
func getEnv() string {
	if env := os.Getenv("APP_ENV"); env != "" {
		return env
	}
	if env := os.Getenv("GO_ENV"); env != "" {
		return env
	}
	return "development" // 默认开发环境
}

// GetMiddlewares 获取当前环境的中间件
func GetMiddlewares() map[string]gin.HandlerFunc {
	// 复制基础中间件
	middlewares := make(map[string]gin.HandlerFunc)
	for k, v := range baseMiddlewares {
		middlewares[k] = v
	}

	// 添加环境特定中间件
	env := getEnv()
	var envSpecific map[string]gin.HandlerFunc

	switch env {
	case "test":
		envSpecific = testMiddlewares
	case "production":
		envSpecific = prodMiddlewares
	default: // development
		envSpecific = devMiddlewares
	}

	for k, v := range envSpecific {
		middlewares[k] = v
	}

	return middlewares
}

// GetMiddlewareStack 获取排序后的中间件切片
func GetMiddlewareStack() []gin.HandlerFunc {
	allMiddlewares := GetMiddlewares()

	// 定义执行顺序（重要！）
	executionOrder := []string{
		"recovery",  // 1. 异常恢复（最外层）
		"requestid", // 2. 请求ID（尽早）
		"logger",    // 3. 日志记录
		"cors",      // 4. 跨域处理
		"secure",    // 5. 安全头
		"nocache",   // 6. 缓存控制
		"dump",      // 7. 调试输出（仅开发环境）
	}

	var stack []gin.HandlerFunc
	for _, key := range executionOrder {
		if middleware, exists := allMiddlewares[key]; exists {
			stack = append(stack, middleware)
		}
	}

	return stack
}
