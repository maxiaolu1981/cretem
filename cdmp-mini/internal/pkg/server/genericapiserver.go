/*
这是一个完整的 Go 语言 HTTP 服务器实现包，基于 Gin Web 框架，提供了通用的 API 服务器功能。以下是详细分析：

核心结构
GenericAPIServer
通用 API 服务器结构体，包含：
insecureServingInfo - HTTP 服务配置信息
insecureServer - HTTP 服务器实例
middlewares - 中间件列表
mode - 服务器模式
enableMetrics - 指标收集开关
enableProfiling - 性能分析开关
healthz - 健康检查开关
*gin.Engine - Gin 引擎实例（继承）

主要方法
1. initGenericAPIServer()
服务器初始化入口，调用三个核心设置方法：

setup() - 基础设置

installMiddlewares() - 中间件安装

installAPIs() - API 路由安装

2. setup()
配置 Gin 路由调试信息输出，使用自定义日志格式

3. installMiddlewares()
安装中间件：

固定安装：RequestID、Context 中间件

动态安装：根据配置列表安装其他中间件

4. installAPIs()
安装API端点：

/healthz - 健康检查端点（如果启用）

Prometheus 指标收集（如果启用）

pprof 性能分析（如果启用）

/version - 版本信息端点

5. Run()
启动服务器的主要方法：

配置 HTTP 服务器

使用 errgroup 进行并发控制

启动健康检查 ping

优雅的错误处理

6. ping()
健康检查方法，在服务器启动后验证路由是否正常加载

技术特性
中间件管理
go
// 预定义的中间件映射
middleware.Middlewares[mw] // 通过名称获取中间件实例
监控集成
Prometheus: 通过 ginprometheus 集成指标收集

pprof: 集成性能分析工具

健康检查: 自定义健康检查端点

错误处理
使用 errgroup 进行并发错误管理，确保所有goroutine正常退出

启动流程
初始化：设置 Gin 引擎和中间件

配置：设置路由调试信息

安装：安装中间件和API路由

启动：创建 HTTP 服务器并监听

验证：执行健康检查 ping

运行：等待服务器运行

健康检查机制
独特的健康检查实现：

go

	func (s *GenericAPIServer) ping(ctx context.Context) error {
	    // 构建检查URL
	    // 循环检查直到成功或超时
	    // 支持本地回环地址转换
	}

错误处理特点
优雅关闭：检查 http.ErrServerClosed 错误

超时控制：使用 context 控制健康检查超时

并发安全：使用 errgroup 管理多个goroutine

详细日志：每个步骤都有详细的日志输出

配置依赖
该服务器依赖多个外部包：

gin - Web 框架

gin-contrib/pprof - 性能分析

go-gin-prometheus - Prometheus 集成

errgroup - 并发控制

自定义中间件和工具包

设计优势
模块化设计：清晰的初始化步骤分离

可扩展性：通过中间件轻松扩展功能

生产就绪：包含监控、健康检查等生产环境必需功能

错误恢复：完善的错误处理和日志记录

标准化响应：使用统一的响应格式

这个包提供了一个完整、健壮的企业级 API 服务器框架，适合构建微服务和 RESTful API。
*/
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type genericAPIServer struct {
	middleware          []string
	insecureServingInfo *insecureServingInfo
	mode                string
	insecureServer      http.Server
	*gin.Engine
	enableProfiling bool
	enableMetrics   bool
	healthz         bool
}
