package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/config"
	genericoptions "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

var _ io.Writer = nil

// 业务错误码定义
const (
	ErrCodeMySQLInit  = 1001 // MySQL初始化失败
	ErrCodeRedisInit  = 1002 // Redis初始化失败
	ErrCodeGRPCStart  = 1003 // gRPC启动失败
	ErrCodeHTTPStart  = 1004 // HTTP服务启动失败
	ErrCodeJWTInvalid = 1005 // JWT配置无效
)

// 全局资源句柄
var (
	mysqlClient  *gorm.DB
	redisClient  *MockRedisClient
	jwtManager   *genericoptions.JwtOptions
	grpcServer   *grpc.Server
	httpServer   *http.Server
	grpcListener net.Listener
)

// ------------------------------
// 核心启动逻辑（实现apiserver.Run）
// ------------------------------

// 重写apiserver.Run，实现服务启动核心逻辑
func Run(cfg *config.Config) error {
	// 创建模块专属日志器
	serverLog := log.WithName("iam-apiserver")
	serverLog.Info("开始初始化IAM API服务器", "version", "v1.0.0")

	// 1. 初始化服务器模式
	if err := initServerMode(cfg.GenericServerRunOptions, serverLog); err != nil {
		return errors.WrapC(err, ErrCodeMySQLInit, "服务器模式初始化失败")
	}

	// 2. 初始化MySQL
	var err error
	mysqlClient, err = initMySQL(cfg.MySQLOptions, serverLog.WithName("mysql"))
	if err != nil {
		serverLog.Error(err, "MySQL初始化失败", "code", ErrCodeMySQLInit)
		return errors.WrapC(err, ErrCodeMySQLInit, "MySQL初始化失败")
	}
	defer func() {
		if sqlDB, err := mysqlClient.DB(); err == nil {
			_ = sqlDB.Close()
		}
		serverLog.Info("MySQL连接已关闭")
	}()

	// 3. 初始化Redis
	redisClient, err = initRedis(cfg.RedisOptions, serverLog.WithName("redis"))
	if err != nil {
		serverLog.Error(err, "Redis初始化失败", "code", ErrCodeRedisInit)
		return errors.WrapC(err, ErrCodeRedisInit, "Redis初始化失败")
	}
	defer func() {
		_ = redisClient.Close()
		serverLog.Info("Redis连接已关闭")
	}()

	// 4. 验证JWT配置
	jwtManager = cfg.JwtOptions
	if err := validateJWT(jwtManager, serverLog.WithName("jwt")); err != nil {
		serverLog.Error(err, "JWT配置验证失败", "code", ErrCodeJWTInvalid)
		return errors.WrapC(err, ErrCodeJWTInvalid, "JWT配置无效")
	}

	// 5. 启动gRPC服务
	if cfg.GRPCOptions.BindPort != 0 { // 通过端口判断是否启用gRPC
		grpcListener, grpcServer, err = startGRPC(cfg.GRPCOptions, serverLog.WithName("grpc"))
		if err != nil {
			serverLog.Error(err, "gRPC服务启动失败", "code", ErrCodeGRPCStart)
			return errors.WrapC(err, ErrCodeGRPCStart, "gRPC服务启动失败")
		}
		defer func() {
			grpcServer.GracefulStop()
			_ = grpcListener.Close()
			serverLog.Info("gRPC服务已关闭")
		}()
	}

	// 6. 初始化HTTP引擎
	engine := initHTTP(cfg.GenericServerRunOptions, serverLog.WithName("http"))

	// 7. 注册路由
	registerRoutes(engine, cfg, serverLog.WithName("routes"))

	// 8. 启动HTTP/HTTPS服务
	httpServer, err = startHTTP(engine, cfg, serverLog.WithName("http"))
	if err != nil {
		serverLog.Error(err, "HTTP服务启动失败", "code", ErrCodeHTTPStart)
		return errors.WrapC(err, ErrCodeHTTPStart, "HTTP服务启动失败")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
		serverLog.Info("HTTP服务已关闭")
	}()

	// 9. 等待中断信号
	serverLog.Info("所有服务启动完成，进入运行状态")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	serverLog.Info("收到中断信号，开始优雅关闭")

	// 刷新日志缓冲区
	log.Flush()
	return nil
}

// ------------------------------
// 初始化辅助函数
// ------------------------------

// initServerMode 初始化服务器运行模式
func initServerMode(opts *genericoptions.ServerRunOptions, logger log.Logger) error {
	logger.Info("初始化服务器模式", "mode", opts.Mode)
	logger.V(1).Info("详细模式配置", "healthz", opts.Healthz, "middlewares", opts.Middlewares)

	switch opts.Mode {
	case "debug":
		gin.SetMode(gin.DebugMode)
	case "test":
		gin.SetMode(gin.TestMode)
	case "release":
		gin.SetMode(gin.ReleaseMode)
	default:
		return errors.New(fmt.Sprintf("不支持的服务器模式: %s", opts.Mode))
	}
	return nil
}

// initMySQL 初始化MySQL连接
func initMySQL(opts *genericoptions.MySQLOptions, logger log.Logger) (*gorm.DB, error) {
	logger.Info("开始初始化MySQL连接", "host", opts.Host, "database", opts.Database)
	if err := opts.Validate(); err != nil {
		return nil, err[0]
	}

	// 调试日志：输出连接参数（脱敏敏感信息）
	logger.V(1).Info("MySQL连接参数",
		"max-open-conns", opts.MaxOpenConnections,
		"max-idle-conns", opts.MaxIdleConnections,
		"life-time", opts.MaxConnectionLifeTime)

	// 调用options包的NewClient创建数据库连接
	db, err := opts.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "创建MySQL客户端失败")
	}
	logger.Info("MySQL连接成功")
	return db, nil
}

// initRedis 初始化Redis连接
func initRedis(opts *genericoptions.RedisOptions, logger log.Logger) (*MockRedisClient, error) {
	logger.Info("开始初始化Redis连接", "host", opts.Host, "port", opts.Port)
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err[0], "Redis配置验证失败")
	}

	// 模拟Redis连接（实际应使用真实客户端）
	if opts.Port == 6380 { // 测试错误场景
		return nil, errors.New("Redis端口被占用")
	}

	client := &MockRedisClient{config: opts}
	logger.Info("Redis连接成功", "db", opts.Database)
	return client, nil
}

// validateJWT 验证JWT配置合法性
func validateJWT(opts *genericoptions.JwtOptions, logger log.Logger) error {
	logger.Info("验证JWT配置", "realm", opts.Realm)
	if err := opts.Validate(); err != nil {
		return errors.Wrap(err[0], "JWT配置验证失败")
	}
	// 敏感信息日志使用V(1)级别
	logger.V(1).Info("JWT密钥验证通过", "key-length", len(opts.Key))
	return nil
}

// startGRPC 启动gRPC服务
func startGRPC(opts *genericoptions.GRPCOptions, logger log.Logger) (net.Listener, *grpc.Server, error) {
	logger.Info("开始启动gRPC服务", "address", opts.BindAddress, "port", opts.BindPort)
	if err := opts.Validate(); err != nil {
		return nil, nil, errors.Wrap(err[0], "gRPC配置验证失败")
	}

	addr := fmt.Sprintf("%s:%d", opts.BindAddress, opts.BindPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "创建gRPC监听器失败")
	}

	// 创建gRPC服务器并配置最大消息大小
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(opts.MaxMsgSize),
		grpc.MaxSendMsgSize(opts.MaxMsgSize),
	)
	// 此处可注册gRPC服务实现（示例省略）

	logger.Info("gRPC服务启动成功", "address", addr)
	go func() {
		if err := srv.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			log.Fatalf("gRPC服务异常退出: %+v", err)
		}
	}()
	return listener, srv, nil
}

// registerRoutes 注册所有路由
func registerRoutes(engine *gin.Engine, cfg *config.Config, logger log.Logger) {
	// 健康检查路由（基于ServerRunOptions.Healthz）
	if cfg.GenericServerRunOptions.Healthz {
		engine.GET("/healthz", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})
		logger.Info("已注册健康检查路由", "path", "/healthz")
	}

	// 性能分析（基于FeatureOptions.EnableProfiling）
	if cfg.FeatureOptions.EnableProfiling {
		pprof.Register(engine)
		logger.Info("已启用性能分析", "path", "/debug/pprof")
	}

	// 指标收集（基于FeatureOptions.EnableMetrics）
	if cfg.FeatureOptions.EnableMetrics {
		engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
		logger.Info("已启用指标收集", "path", "/metrics")
	}

	// IAM核心API路由（带JWT认证）
	v1 := engine.Group("/api/v1")
	v1.Use(jwtAuthMiddleware(jwtManager, logger.WithName("jwt-auth")))
	{
		users := v1.Group("/users")
		{
			users.GET("", listUsers)   // 列出用户
			users.GET("/:id", getUser) // 获取用户详情
			users.POST("", createUser) // 创建用户
		}
		logger.Info("已注册IAM核心API路由", "prefix", "/api/v1")
	}
}

// startHTTP 启动HTTP/HTTPS服务
func startHTTP(engine *gin.Engine, cfg *config.Config, logger log.Logger) (*http.Server, error) {
	var srv *http.Server

	// 优先启动HTTPS服务（基于SecureServing配置）
	if cfg.SecureServing.BindPort != 0 {
		addr := fmt.Sprintf("%s:%d", cfg.SecureServing.BindAddress, cfg.SecureServing.BindPort)
		srv = &http.Server{Addr: addr, Handler: engine}
		logger.Info("启动HTTPS服务", "address", addr, "cert", cfg.SecureServing.ServerCert.CertKey.CertFile)

		go func() {
			if err := srv.ListenAndServeTLS(
				cfg.SecureServing.ServerCert.CertKey.CertFile,
				cfg.SecureServing.ServerCert.CertKey.KeyFile,
			); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS服务异常: %+v", err)
			}
		}()
		return srv, nil
	}

	// 启动HTTP服务（基于InsecureServing配置）
	addr := fmt.Sprintf("%s:%d", cfg.InsecureServing.BindAddress, cfg.InsecureServing.BindPort)
	srv = &http.Server{Addr: addr, Handler: engine}
	logger.Info("启动HTTP服务", "address", addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP服务异常: %+v", err)
		}
	}()
	return srv, nil
}

// ------------------------------
// 业务逻辑与中间件
// ------------------------------

// jwtAuthMiddleware JWT认证中间件
func jwtAuthMiddleware(manager *genericoptions.JwtOptions, logger log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			err := errors.New("缺少Authorization令牌")
			// 使用V(1)级别记录调试日志
			logger.WithValues("client-ip", c.ClientIP()).V(1).Info("认证失败", "error", err.Error())
			// 记录错误日志
			logger.Error(err, "认证失败", "client-ip", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error(), "code": 401})
			c.Abort()
			return
		}

		// 模拟令牌验证（实际应使用manager.Key验证）
		if token != "valid-token" {
			err := errors.WithCode(401, "无效的JWT令牌")
			logger.Error(err, "令牌验证失败", "token", token, "client-ip", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error(), "code": 401})
			c.Abort()
			return
		}

		c.Next()
	}
}

// 模拟API处理函数
func listUsers(c *gin.Context) {
	// 从上下文获取日志器（假设log包实现了FromContext）
	log.FromContext(c).Info("查询用户列表", "page", c.Query("page"))
	c.JSON(http.StatusOK, []map[string]interface{}{
		{"id": "1", "name": "admin"},
		{"id": "2", "name": "user1"},
	})
}

func getUser(c *gin.Context) {
	userID := c.Param("id")
	log.FromContext(c).V(1).Info("查询用户详情", "user-id", userID)
	c.JSON(http.StatusOK, map[string]interface{}{"id": userID, "name": "admin"})
}

func createUser(c *gin.Context) {
	var req struct{ Name string }
	if err := c.ShouldBindJSON(&req); err != nil {
		wrappedErr := errors.Wrap(err, "请求参数无效")
		log.FromContext(c).Error(wrappedErr, "创建用户失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": wrappedErr.Error()})
		return
	}
	c.JSON(http.StatusCreated, map[string]interface{}{"id": "3", "name": req.Name})
}

// ------------------------------
// 模拟客户端与工具
// ------------------------------

// MockRedisClient 模拟Redis客户端
type MockRedisClient struct {
	config *genericoptions.RedisOptions
}

func (c *MockRedisClient) Close() error {
	return nil
}

// ------------------------------
// 程序入口
// ------------------------------

func main() {
	// 模拟命令行参数（实际由apiserver包解析）
	os.Args = []string{
		"iam-apiserver",
		"--server.mode=debug",
		"--server.healthz=true",
		"--server.middlewares=recovery,logger",
		"--feature.profiling=true",
		"--feature.enable-metrics=true",
		"--insecure.bind-address=0.0.0.0",
		"--insecure.bind-port=8080",
		"--secure.bind-port=0", // 禁用HTTPS
		"--grpc.bind-port=9090",
		"--mysql.host=127.0.0.1",
		"--mysql.port=3306",
		"--mysql.database=iam",
		"--redis.host=127.0.0.1",
		"--redis.port=6379",
		"--jwt.key=valid-jwt-key-32bytes!",
		"--log.level=info",
		"--log.format=json",
	}

	// 启动应用（通过apiserver包创建应用实例）
	app := apiserver.NewApp("iam-apiserver")
	app.Run()

	// 退出前刷新日志
	log.Flush()
}

func initHTTP(opts *genericoptions.ServerRunOptions, logger log.Logger) *gin.Engine {
	logger.Info("初始化HTTP引擎", "middlewares", opts.Middlewares)
	engine := gin.New()

	// 注册中间件（确保for循环和switch完全闭合）
	for _, mid := range opts.Middlewares {
		switch mid {
		case "recovery":
			engine.Use(gin.Recovery())
			logger.Info("已注册中间件", "name", "recovery")
		case "logger":
			//  logWriter := io.WriterFunc(func(p []byte) (int, error) {
			// 	logMsg := strings.TrimSpace(string(p))
			// 	if logMsg != "" {
			// 		logger.Info("HTTP请求日志", "details", logMsg)
			// 	}
			// 	return len(p), nil})
		default:
			log.Warnw("未知中间件", "name", mid)
		}
	} // for循环结束（修复：正确闭合循环）
	return engine // 修复：在循环外返回
} // 修复：函数正确闭合
