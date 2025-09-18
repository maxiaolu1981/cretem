package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/control/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business/auth"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

func (g *GenericAPIServer) installRoutes() error {
	// 系统路由（最先注册，通常无需认证）
	if err := g.installSystemRoutes(); err != nil {
		return err
	}

	// 认证路由
	if err := g.installAuthRoutes(); err != nil {
		return err
	}

	// 公共路由（部分公开API）- 业务级别但无需认证
	//if err := installPublicRoutes(); err != nil {
	//	return err
	//}

	// API路由（需要用户认证）
	if err := g.installApiRoutes(); err != nil {
		return err
	}

	// 管理路由（需要管理员认证）- 系统管理功能
	//if err := installAdminRoutes(engine, opts); err != nil {
	//	return err
	//.}

	// 6. 内部路由（内部服务调用）- 服务间通信
	//if err := installInternalRoutes(engine, opts); err != nil {
	//		return err
	//	}

	return nil
}

func (g *GenericAPIServer) installSystemRoutes() error {

	if g.options.ServerRunOptions.Healthz {
		g.GET("/healthz", func(c *gin.Context) {
			core.WriteResponse(c, nil, map[string]string{
				"status": "ok"})
		})
	}

	g.GET("/test-all-metrics", func(c *gin.Context) {
		// 测试所有业务指标
		metrics.BusinessSuccess.WithLabelValues("test_create").Inc()
		metrics.BusinessSuccess.WithLabelValues("test_update").Inc()
		metrics.BusinessFailures.WithLabelValues("test_create", "timeout").Inc()
		metrics.BusinessProcessingTime.WithLabelValues("test_create").Observe(0.15)
		metrics.BusinessProcessingTime.WithLabelValues("test_update").Observe(0.08)

		// 测试所有生产者指标
		metrics.ProducerAttempts.WithLabelValues("user.create.v1", "create").Inc()
		metrics.ProducerAttempts.WithLabelValues("user.update.v1", "update").Inc()
		metrics.ProducerSuccess.WithLabelValues("user.create.v1", "create").Inc()
		metrics.ProducerFailures.WithLabelValues("user.create.v1", "create", "network_error").Inc()
		metrics.ProducerRetries.WithLabelValues("user.create.v1", "create").Inc()
		metrics.DeadLetterMessages.WithLabelValues("user.create.v1", "create").Inc()
		metrics.MessageProcessingTime.WithLabelValues("user.create.v1", "create", "success").Observe(0.2)

		// 测试消费者指标（如果有消费者的话）
		metrics.ConsumerMessagesReceived.WithLabelValues("user.create.v1", "user-service").Inc()
		metrics.ConsumerMessagesProcessed.WithLabelValues("user.create.v1", "user-service").Inc()
		metrics.ConsumerProcessingErrors.WithLabelValues("user.create.v1", "user-service", "decode_error").Inc()

		c.JSON(200, gin.H{
			"message": "所有指标已触发",
			"metrics": []string{
				"business_operations_success_total",
				"business_operations_failures_total",
				"business_processing_seconds",
				"kafka_producer_attempts_total",
				"kafka_producer_success_total",
				"kafka_producer_failures_total",
				"kafka_producer_retries_total",
				"kafka_dead_letter_messages_total",
				"kafka_message_processing_seconds",
				"kafka_consumer_messages_received_total",
				"kafka_consumer_messages_processed_total",
				"kafka_consumer_processing_errors_total",
			},
		})
	})

	if g.options.ServerRunOptions.EnableMetrics {
		prometheus := ginprometheus.NewPrometheus("gin")
		prometheus.Use(g.Engine)
	}
	if g.options.ServerRunOptions.EnableProfiling && g.options.ServerRunOptions.Mode == gin.DebugMode {
		pprof.Register(g)
	}
	g.GET("/version", func(c *gin.Context) {
		core.WriteResponse(c, nil, version.Get().ToJSON())
	})
	return nil
}

func (g *GenericAPIServer) installAuthRoutes() error {
	jwtStrategy, err := g.newJWTAuth()
	if err != nil {
		return err
	}
	jwt, ok := jwtStrategy.(auth.JWTStrategy)
	if !ok {
		return fmt.Errorf("转换jwtStrategy错误")
	}

	//loginLimiter := common.LoginRateLimiter(g.redis, 200, time.Second)
	// 登录：使用 gin-jwt 的 LoginHandler（需要认证中间件）
	g.Handle(http.MethodPost, "/login",
		createAutHandler(), // 然后是认证相关的中间件
		jwt.LoginHandler,   // 最后是实际的登录处理函数
	)
	// g.Handle(http.MethodPost, "/login",
	// 	loginLimiter,       // 限流放在最前面
	// 	createAutHandler(), // 然后是认证相关的中间件
	// 	jwt.LoginHandler,   // 最后是实际的登录处理函数
	// )

	g.POST("logout", createAutHandler(), g.logoutRespons)
	// 刷新：使用 gin-jwt 的 RefreshHandler（需要认证中间件
	g.POST("/refresh", g.ValidateATMiddleware(), g.ValidateATForRefreshMiddleware)

	return nil
}

func (g *GenericAPIServer) installApiRoutes() error {
	auto, err := g.newAutoAuth()
	if err != nil {
		return err
	}
	g.NoRoute(func(c *gin.Context) {
		core.WriteResponse(
			c,
			errors.WithCode(code.ErrPageNotFound, "业务不存在"),
			nil,
		)

	})
	storeIns, _, _ := store.GetMySQLFactoryOr(nil)
	v1 := g.Group("/v1")
	{
		userv1 := v1.Group("/users")
		{
			userController, err := user.NewUserController(storeIns,
				g.redis, g.options,
				g.BloomFilter,
				&g.BloomMutex,
				g.producer,
			)
			if err != nil {
				return err
			}
			userv1.Use(auto.AuthFunc(), middleware.Validation(g.options))
			userv1.DELETE(":name", userController.Delete)
			userv1.DELETE(":name/force", userController.ForceDelete)
			userv1.POST("", userController.Create)
			userv1.GET(":name", userController.Get)
			userv1.GET("", userController.List)
		}

	}
	return nil
}

func createAutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		contentType := c.ContentType()
		if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
			// 表单格式会触发此逻辑，返回 415 + 100007
			core.WriteResponse(c, errors.WithCode(code.ErrUnsupportedMediaType, "不支持的Content-Type..."), nil)
			c.Abort()
			return
		}
		rawAuthHeader := c.GetHeader("Authorization")

		c.Set("raw_auth_header", rawAuthHeader)
		//jwtHandler(c) // 格式正确才进入实际登录逻辑
		c.Next()
	}
}
