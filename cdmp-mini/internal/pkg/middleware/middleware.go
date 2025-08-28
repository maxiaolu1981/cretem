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
