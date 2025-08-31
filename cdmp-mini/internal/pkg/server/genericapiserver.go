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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"golang.org/x/sync/errgroup"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

type GenericAPIServer struct {
	insecureServer *http.Server
	*gin.Engine
	options *options.Options
}

func NewGenericAPIServer(opts *options.Options) (*GenericAPIServer, error) {
	// 初始化日志
	log.Infof("正在初始化GenericAPIServer服务器，环境: %s", opts.ServerRunOptions.Mode)

	//创建服务器实例
	g := &GenericAPIServer{
		Engine:  gin.New(),
		options: opts,
	}

	//设置gin运行模式
	if err := g.configureGin(); err != nil {
		return nil, err
	}

	//安装中间件
	if err := middleware.InstallMiddlewares(g.Engine, opts); err != nil {
		return nil, err
	}
	//. 安装路由
	installRoutes(g.Engine, opts)

	return g, nil
}

func (g *GenericAPIServer) configureGin() error {
	// 设置运行模式
	gin.SetMode(g.options.ServerRunOptions.Mode)

	// 开发环境配置
	if g.options.ServerRunOptions.Mode == gin.DebugMode {
		gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
			log.Debugf("📍 %-6s %-50s → %s (%d middleware)",
				httpMethod, absolutePath, filepath.Base(handlerName), nuHandlers)
		}
	} else {
		// 生产环境禁用调试输出
		gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {}
	}

	return nil
}

func (g *GenericAPIServer) Run() error {
	address := net.JoinHostPort(g.options.InsecureServingOptions.BindAddress, strconv.Itoa((g.options.InsecureServingOptions.BindPort)))

	g.insecureServer = &http.Server{
		Addr:    address,
		Handler: g,
	}

	var eg errgroup.Group

	// 创建服务器启动信号通道
	serverStarted := make(chan struct{})

	eg.Go(func() error {
		log.Infof("正在 %s 启动 GenericAPIServer 服务", address)

		// 创建监听器，确保端口可用
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("创建监听器失败: %w", err)
		}

		log.Info("端口监听成功，开始接受连接")
		close(serverStarted)

		// 启动服务器
		err = g.insecureServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			log.Infof("GenericAPIServer服务器已正常关闭")
			return nil
		}
		if err != nil {
			return fmt.Errorf("GenericAPIServer服务器启动失败: %w", err)
		}

		log.Infof("停止 %s 运行的 GenericAPIServer 服务", address)
		return nil
	})

	// 等待服务器开始监听
	select {
	case <-serverStarted:
		log.Info("GenericAPIServer服务器已开始监听，准备进行健康检查...")
	case <-time.After(5 * time.Second):
		return fmt.Errorf("GenericAPIServer服务器启动超时，无法在5秒内开始监听")
	}

	if g.options.ServerRunOptions.Healthz {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 先等待端口就绪
		if err := g.waitForPortReady(ctx, address, 10*time.Second); err != nil {
			return fmt.Errorf("端口就绪检测失败: %w", err)
		}

		// 执行健康检查
		if err := g.ping(ctx, address); err != nil {
			return fmt.Errorf("健康检查失败: %w", err)
		}
	}

	// 添加最终的成功日志
	log.Infof("✨ GenericAPIServer 服务已在 %s 成功启动并运行", address)

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("服务器运行错误: %w", err)
	}
	return nil
}

// waitForPortReady 等待端口就绪
func (g *GenericAPIServer) waitForPortReady(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	log.Infof("等待端口 %s 就绪，超时时间: %v", address, timeout)

	for attempt := 1; ; attempt++ {
		// 检查是否超时
		if time.Now().After(deadline) {
			return fmt.Errorf("端口就绪检测超时")
		}

		// 尝试连接端口
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			log.Infof("端口 %s 就绪检测成功，尝试次数: %d", address, attempt)
			return nil
		}

		// 记录重试信息（每5次尝试记录一次）
		if attempt%5 == 0 {
			log.Infof("端口就绪检测尝试 %d: %v", attempt, err)
		}

		// 等待重试或上下文取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("端口就绪检测被取消: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
			// 继续重试
		}
	}
}

func (g *GenericAPIServer) ping(ctx context.Context, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("无效的地址格式: %w", err)
	}

	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	url := fmt.Sprintf("http://%s/healthz", net.JoinHostPort(host, port))
	log.Infof("开始健康检查，目标URL: %s", url)

	startTime := time.Now()
	attempt := 0

	for {
		attempt++
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("健康检查超时: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("创建请求失败: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if attempt%3 == 0 { // 每3次失败记录一次日志，避免日志过多
				log.Infof("健康检查尝试 %d 失败: %v", attempt, err)
			}
		} else {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Infof("健康检查成功! 总共尝试 %d 次, 耗时 %v",
					attempt, time.Since(startTime))
				return nil
			}

			log.Infof("健康检查尝试 %d: 状态码 %d", attempt, resp.StatusCode)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("健康检查超时: %w", ctx.Err())
		case <-time.After(1 * time.Second):
			// 继续重试
		}
	}
}
