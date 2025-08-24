/*
server 包实现了一个基于 Gin 框架的通用 API 服务器（GenericAPIServer），支持以下核心功能：
同时启动 HTTP（非安全）和 HTTPS（安全）服务；
集成健康检查（/healthz）、版本查询（/version）、性能分析（pprof）和指标收集（Prometheus）；
支持自定义中间件安装；
实现服务器的优雅启动与关闭（通过 errgroup 管理服务协程，http.Server.Shutdown 处理关闭）
核心流程
GenericAPIServer 的生命周期可分为 初始化→启动→运行→关闭 四个阶段，具体流程如下：
初始化阶段
通过 initGenericAPIServer 完成基础设置：
Setup()：配置 Gin 路由日志打印格式；
InstallMiddlewares()：安装必要中间件（如请求 ID、上下文管理）和自定义中间件；
InstallAPIs()：注册基础接口（健康检查、版本查询、pprof、Prometheus 指标）。
启动阶段（Run 方法）
初始化 HTTP 服务器（insecureServer）和 HTTPS 服务器（secureServer），分别绑定配置的地址和端口；
使用 errgroup 并发启动两个服务器（HTTP 和 HTTPS），避免阻塞；
启动后通过 ping 方法检查服务器是否正常（访问 /healthz 接口）。
运行阶段
服务器持续监听并处理客户端请求，依托 Gin 框架的路由和中间件完成请求分发；
支持通过 pprof 进行性能分析（如 /-/pprof）、通过 Prometheus 收集指标（默认 /metrics）。
关闭阶段（Close 方法）
调用 http.Server.Shutdown 优雅关闭 HTTP 和 HTTPS 服务，确保正在处理的请求完成后再终止；
超时控制（默认 10 秒），避免关闭过程无限阻塞。
*/
package server

import (
	"context"  // 标准库：提供上下文管理（用于超时控制）
	"errors"   // 标准库：错误处理
	"fmt"      // 标准库：格式化输出
	"net/http" // 标准库：HTTP 服务器基础功能
	"strings"  // 标准库：字符串处理
	"time"     // 标准库：时间相关操作

	"golang.org/x/sync/errgroup" // 第三方库：用于管理并发任务的错误组

	"github.com/gin-contrib/pprof"                     // 第三方库：Gin 集成 pprof（性能分析）
	"github.com/gin-gonic/gin"                         // 第三方库：Gin Web 框架
	ginprometheus "github.com/zsais/go-gin-prometheus" // 第三方库：Gin 集成 Prometheus（指标收集）

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware" // 自定义库：项目自定义中间件
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"        // 自定义库：基础组件（响应处理）
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"     // 自定义库：版本信息管理
	"github.com/maxiaolu1981/cretem/nexuscore/log"                        // 自定义库：日志工具
)

// GenericAPIServer 包含 API 服务器的核心状态，基于 Gin 框架扩展
type GenericAPIServer struct {
	middlewares []string // 需要安装的自定义中间件名称列表

	// SecureServingInfo 安全服务（HTTPS）的配置信息
	SecureServingInfo *SecureServingInfo
	// InsecureServingInfo 非安全服务（HTTP）的配置信息
	InsecureServingInfo *InsecureServingInfo

	// ShutdownTimeout 服务器关闭的超时时间（优雅关闭的最大等待时间）
	ShutdownTimeout time.Duration

	*gin.Engine          // 嵌入 Gin 引擎，继承其路由和处理能力
	healthz         bool // 是否启用健康检查接口（/healthz）
	enableMetrics   bool // 是否启用 Prometheus 指标收集
	enableProfiling bool // 是否启用 pprof 性能分析

	insecureServer *http.Server // HTTP 服务器实例
	secureServer   *http.Server // HTTPS 服务器实例
}

// initGenericAPIServer 初始化 GenericAPIServer，完成基础设置
func initGenericAPIServer(s *GenericAPIServer) {
	s.Setup()              // 配置 Gin 框架基础参数
	s.InstallMiddlewares() // 安装中间件
	s.InstallAPIs()        // 注册基础 API 接口
}

// InstallAPIs 注册通用 API 接口（健康检查、版本、指标等）
func (s *GenericAPIServer) InstallAPIs() {
	// 注册健康检查接口（/healthz）
	if s.healthz {
		s.GET("/healthz", func(c *gin.Context) {
			core.WriteResponse(c, nil, map[string]string{"status": "ok"})
		})
	}

	// 注册 Prometheus 指标收集（默认暴露 /metrics 接口）
	if s.enableMetrics {
		prometheus := ginprometheus.NewPrometheus("gin") // 以 "gin" 为指标前缀
		prometheus.Use(s.Engine)                         // 将指标中间件添加到 Gin 引擎
	}

	// 注册 pprof 性能分析接口（如 /debug/pprof）
	if s.enableProfiling {
		pprof.Register(s.Engine) // 注册 pprof 路由到 Gin 引擎
	}

	// 注册版本查询接口（/version）
	s.GET("/version", func(c *gin.Context) {
		core.WriteResponse(c, nil, version.Get()) // 返回项目版本信息
	})
}

// Setup 配置 Gin 引擎的基础参数（如路由日志格式）
func (s *GenericAPIServer) Setup() {
	// 自定义 Gin 路由注册日志格式：打印请求方法、路径、处理器名称和处理器数量
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %-s --> %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

// InstallMiddlewares 安装中间件（基础中间件 + 自定义中间件）
func (s *GenericAPIServer) InstallMiddlewares() {
	// 安装必要的基础中间件
	s.Use(middleware.RequestID()) // 生成并注入请求 ID（用于追踪）
	s.Use(middleware.Context())   // 注入上下文（如日志、超时控制）

	// 安装自定义中间件（从 middleware 包中查找并注册）
	for _, m := range s.middlewares {
		mw, ok := middleware.Middlewares[m] // 从全局中间件映射中获取
		if !ok {
			log.Warnf("未找到中间件: %s", m)
			continue
		}

		log.Infof("安装中间件: %s", m)
		s.Use(mw) // 将中间件添加到 Gin 引擎
	}
}

// Run 启动 HTTP 和 HTTPS 服务器（阻塞方法，直到服务终止）
func (s *GenericAPIServer) Run() error {
	// 初始化 HTTP 服务器（非安全）
	s.insecureServer = &http.Server{
		Addr:           s.InsecureServingInfo.Address, // 绑定的地址（如 "0.0.0.0:8080"）
		Handler:        s,                             // 处理请求的处理器（即当前 GenericAPIServer，继承自 Gin.Engine）
		ReadTimeout:    10 * time.Second,              //限制从客户端建立连接到服务器读完整个请求（包括请求体） 的最大时间，防止恶意客户端长时间保持连接不发送数据，耗尽服务器资源。
		WriteTimeout:   30 * time.Second,              //限制从服务器读完请求到发送完整个响应（包括响应体） 的最大时间，防止处理耗时过长的请求阻塞服务器（如复杂计算、慢查询导致的响应延迟）。
		MaxHeaderBytes: 1 << 20,                       //限制 HTTP 请求头的总大小，防止恶意客户端发送超大请求头（如 Cookie、自定义头）导致内存溢出或 DoS 攻击。
	}

	// 初始化 HTTPS 服务器（安全）
	s.secureServer = &http.Server{
		Addr:    s.SecureServingInfo.Address(), // 绑定的地址（如 "0.0.0.0:8443"）
		Handler: s,                             // 处理请求的处理器
	}

	// 使用 errgroup 管理并发的服务器协程（统一处理错误）
	var eg errgroup.Group

	// 启动 HTTP 服务器（协程中运行，避免阻塞）
	eg.Go(func() error {
		log.Infof("开始监听 HTTP 请求，地址: %s", s.InsecureServingInfo.Address)

		// 启动 HTTP 服务，若失败且非正常关闭（http.ErrServerClosed），则返回错误
		if err := s.insecureServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())
			return err
		}

		log.Infof("HTTP 服务器已停止，地址: %s", s.InsecureServingInfo.Address)
		return nil
	})

	// 启动 HTTPS 服务器（协程中运行）
	eg.Go(func() error {
		// 检查 HTTPS 配置是否完整（证书和端口必填）
		key, cert := s.SecureServingInfo.CertKey.KeyFile, s.SecureServingInfo.CertKey.CertFile
		if cert == "" || key == "" || s.SecureServingInfo.BindPort == 0 {
			return nil // 配置不完整，不启动 HTTPS 服务
		}

		log.Infof("开始监听 HTTPS 请求，地址: %s", s.SecureServingInfo.Address())

		// 启动 HTTPS 服务（使用指定的证书和密钥）
		if err := s.secureServer.ListenAndServeTLS(cert, key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())
			return err
		}

		log.Infof("HTTPS 服务器已停止，地址: %s", s.SecureServingInfo.Address())
		return nil
	})

	// 检查服务器是否正常启动（通过访问 /healthz 接口）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.healthz {
		if err := s.ping(ctx); err != nil {
			return err
		}
	}

	// 等待所有服务器协程结束（若有错误则返回）
	if err := eg.Wait(); err != nil {
		log.Fatal(err.Error())
	}

	return nil
}

// Close 优雅关闭服务器（等待正在处理的请求完成后终止）
func (s *GenericAPIServer) Close() {
	// 创建带超时的上下文（控制关闭的最大等待时间）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 关闭 HTTPS 服务器
	if err := s.secureServer.Shutdown(ctx); err != nil {
		log.Warnf("关闭 HTTPS 服务器失败: %s", err.Error())
	}

	// 关闭 HTTP 服务器
	if err := s.insecureServer.Shutdown(ctx); err != nil {
		log.Warnf("关闭 HTTP 服务器失败: %s", err.Error())
	}
}

// ping 检查服务器是否正常启动（通过访问 /healthz 接口）
func (s *GenericAPIServer) ping(ctx context.Context) error {
	// 构建 /healthz 接口的访问地址
	url := fmt.Sprintf("http://%s/healthz", s.InsecureServingInfo.Address)
	// 若地址包含 0.0.0.0（监听所有网卡），则替换为 127.0.0.1 本地访问
	if strings.Contains(s.InsecureServingInfo.Address, "0.0.0.0") {
		port := strings.Split(s.InsecureServingInfo.Address, ":")[1]
		url = fmt.Sprintf("http://127.0.0.1:%s/healthz", port)
	}

	// 循环检查，直到成功或超时
	for {
		// 创建带上下文的 HTTP 请求（支持超时控制）
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		// 发送请求检查健康状态
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Info("路由已成功部署。")
			resp.Body.Close()
			return nil
		}

		// 未成功，1 秒后重试
		log.Info("等待路由启动，1 秒后重试...")
		time.Sleep(1 * time.Second)

		// 检查是否超时
		select {
		case <-ctx.Done():
			log.Fatal("在指定时间内无法连接到 HTTP 服务器。")
		default:
		}
	}
}
