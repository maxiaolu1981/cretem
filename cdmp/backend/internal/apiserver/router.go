// 包 apiserver 负责 API 路由注册与中间件安装。
// 主要流程：
// 1. 初始化 gin 路由引擎。
// 2. 安装认证、校验等中间件。
// 3. 注册各类 RESTful 控制器（用户、策略、密钥等）。
// 4. 支持 JWT 登录、刷新、登出等接口。
// 5. 所有 /v1 路径下接口均需认证。
//
// 用法示例：
//
//	r := gin.New()
//	initRouter(r)
//	r.Run()
package apiserver

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/controller/v1/cache/policy"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/controller/v1/secret"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/controller/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store/mysql"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware/auth"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	// custom gin validators.
	_ "github.com/maxiaolu1981/cretem/cdmp/backend/pkg/validator"
)

// 初始化路由，安装中间件和控制器。
func initRouter(g *gin.Engine) {
	installMiddleware(g)
	installController(g)
}

// 安装全局中间件（如认证、日志等）。
func installMiddleware(g *gin.Engine) {
	// 可在此添加全局中间件
}

// 注册所有控制器和路由。
func installController(g *gin.Engine) *gin.Engine {
	// 认证相关路由
	jwtStrategy, _ := newJWTAuth().(auth.JWTStrategy)
	g.POST("/login", jwtStrategy.LoginHandler)     // 登录接口
	g.POST("/logout", jwtStrategy.LogoutHandler)   // 登出接口
	g.POST("/refresh", jwtStrategy.RefreshHandler) // 刷新 token 接口

	// 未匹配路由处理，带认证
	auto := newAutoAuth()
	g.NoRoute(auto.AuthFunc(), func(c *gin.Context) {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "Page not found."), nil)
	})

	// v1 版本接口，需认证
	storeIns, _ := mysql.GetMySQLFactoryOr(nil)
	v1 := g.Group("/v1")
	{
		// 用户相关 RESTful 路由
		userv1 := v1.Group("/users")
		{
			userController := user.NewUserController(storeIns)

			userv1.POST("", userController.Create)               // 创建用户
			userv1.Use(auto.AuthFunc(), middleware.Validation()) // 用户接口认证与校验
			// userv1.PUT("/find_password", userController.FindPassword) // 找回密码（注释掉）
			userv1.DELETE("", userController.DeleteCollection)                 // 批量删除（管理员）
			userv1.DELETE(":name", userController.Delete)                      // 删除单个用户（管理员）
			userv1.PUT(":name/change-password", userController.ChangePassword) // 修改密码
			userv1.PUT(":name", userController.Update)                         // 更新用户信息
			userv1.GET("", userController.List)                                // 用户列表
			userv1.GET(":name", userController.Get)                            // 获取用户详情（管理员）
		}

		v1.Use(auto.AuthFunc()) // v1下所有接口需认证

		// 策略相关 RESTful 路由
		policyv1 := v1.Group("/policies", middleware.Publish())
		{
			policyController := policy.NewPolicyController(storeIns)

			policyv1.POST("", policyController.Create)             // 创建策略
			policyv1.DELETE("", policyController.DeleteCollection) // 批量删除
			policyv1.DELETE(":name", policyController.Delete)      // 删除单个策略
			policyv1.PUT(":name", policyController.Update)         // 更新策略
			policyv1.GET("", policyController.List)                // 策略列表
			policyv1.GET(":name", policyController.Get)            // 获取策略详情
		}

		// 密钥相关 RESTful 路由
		secretv1 := v1.Group("/secrets", middleware.Publish())
		{
			secretController := secret.NewSecretController(storeIns)

			secretv1.POST("", secretController.Create)        // 创建密钥
			secretv1.DELETE(":name", secretController.Delete) // 删除密钥
			secretv1.PUT(":name", secretController.Update)    // 更新密钥
			secretv1.GET("", secretController.List)           // 密钥列表
			secretv1.GET(":name", secretController.Get)       // 获取密钥详情
		}
	}

	return g
}
