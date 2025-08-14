/*
包摘要
该包（package middleware）定义了一系列通用的 Gin 中间件，涵盖缓存控制、跨域处理、安全增强、请求追踪等功能，并通过 Middlewares 变量统一管理所有注册的中间件，方便在项目中按需引入。这些中间件主要用于规范 HTTP 头信息、增强接口安全性、处理跨域请求等基础共性需求。
核心流程
中间件定义：分别实现 NoCache（禁用缓存）、Options（处理跨域预检请求）、Secure（添加安全头）等中间件，每个中间件专注于特定的 HTTP 头设置或请求处理逻辑。
统一管理：通过 defaultMiddlewares 函数初始化 Middlewares 变量，将内置中间件（如 gin.Recovery()）和自定义中间件（如 RequestID、Cors）以键值对形式存储，便于路由注册时快速引用。
执行逻辑：中间件通过修改 gin.Context 的响应头或处理特定请求（如 OPTIONS 方法），在请求到达业务处理器前完成基础配置，再通过 c.Next() 传递请求或 c.Abort() 终止流程。


*/

package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	gindump "github.com/tpkeeper/gin-dump" // 用于请求/响应内容 dump 的第三方中间件
)

// Middlewares 存储所有已注册的中间件，以键值对形式管理（键为中间件名称，值为中间件函数）
var Middlewares = defaultMiddlewares()

// NoCache 是一个中间件，用于添加响应头以阻止客户端缓存 HTTP 响应
func NoCache(c *gin.Context) {
	// 设置 Cache-Control：禁止缓存、存储，强制重新验证
	c.Header("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate, value")
	// 设置 Expires：过期时间设为 1970 年，强制客户端认为响应已过期
	c.Header("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
	// 设置 Last-Modified：以当前时间为最后修改时间，使客户端每次请求都验证新鲜度
	c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	// 继续执行下一个中间件或业务处理器
	c.Next()
}

// Options 是一个中间件，用于处理 OPTIONS 预检请求（跨域场景），
// 并在处理后终止中间件链（避免传递给业务处理器）
func Options(c *gin.Context) {
	// 若请求方法不是 OPTIONS，则继续执行后续逻辑
	if c.Request.Method != "OPTIONS" {
		c.Next()
	} else {
		// 处理 OPTIONS 请求：设置跨域相关响应头
		c.Header("Access-Control-Allow-Origin", "*")                                            // 允许所有源跨域
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")           // 允许的 HTTP 方法
		c.Header("Access-Control-Allow-Headers", "authorization, origin, content-type, accept") // 允许的请求头
		c.Header("Allow", "HEAD,GET,POST,PUT,PATCH,DELETE,OPTIONS")                             // 服务器支持的方法
		c.Header("Content-Type", "application/json")                                            // 响应类型为 JSON
		// 终止请求处理，返回 200 状态码（不执行后续中间件和处理器）
		c.AbortWithStatus(http.StatusOK)
	}
}

// Secure 是一个中间件，用于添加安全相关和资源访问控制的响应头，增强接口安全性
func Secure(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")  // 允许所有源跨域（与 Options 中间件呼应）
	c.Header("X-Frame-Options", "DENY")           // 禁止页面被嵌入 iframe（防止点击劫持）
	c.Header("X-Content-Type-Options", "nosniff") // 禁止浏览器猜测 MIME 类型（防止 XSS）
	c.Header("X-XSS-Protection", "1; mode=block") // 启用 XSS 过滤，检测到攻击时阻止页面加载

	// 若请求使用 HTTPS（TLS 加密），添加 HSTS 头（强制后续请求使用 HTTPS）
	if c.Request.TLS != nil {
		c.Header("Strict-Transport-Security", "max-age=31536000") // 有效期 1 年
	}

	// 继续执行下一个中间件或业务处理器
	c.Next()
}

// defaultMiddlewares 初始化默认中间件集合，返回包含内置和自定义中间件的映射
func defaultMiddlewares() map[string]gin.HandlerFunc {
	return map[string]gin.HandlerFunc{
		"recovery":  gin.Recovery(), // Gin 内置中间件：捕获 panic 并返回 500 错误
		"secure":    Secure,         // 自定义安全头中间件
		"options":   Options,        // 自定义 OPTIONS 请求处理中间件
		"nocache":   NoCache,        // 自定义禁用缓存中间件
		"cors":      Cors(),         // 自定义跨域处理中间件（需在其他地方实现）
		"requestid": RequestID(),    // 自定义请求 ID 中间件（前文定义的 RequestID 函数）
		"logger":    Logger(),       // 自定义日志中间件（需在其他地方实现）
		"dump":      gindump.Dump(), // 第三方中间件：打印请求/响应详情（调试用）
	}
}
