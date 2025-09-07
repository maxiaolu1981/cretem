/*
这个包是IAM项目的API服务器路由配置模块，负责定义所有HTTP路由和中间件设置。以下是包摘要：
1.包功能
路由初始化: 设置Gin引擎的所有路由和处理器
中间件安装: 配置认证、验证等中间件
控制器注册: 注册所有RESTful API控制器
2.核心函数
initRouter(g *gin.Engine)
主路由初始化函数，调用中间件和控制器安装
installMiddleware(g *gin.Engine)
中间件安装函数（当前为空实现）
installController(g *gin.Engine) *gin.Engine
控制器安装函数，包含所有路由定义
3.路由结构
1.)认证路由
POST /login - JWT登录处理
POST /logout - JWT登出处理
POST /refresh - JWT令牌刷新
2.)API版本分组 /v1
用户管理 /users
POST /users - 创建用户（无需认证）
DELETE /users - 批量删除用户（管理员）
DELETE /users:name - 删除用户（管理员）
PUT /users:name/change-password - 修改密码
PUT /users:name - 更新用户信息
GET /users - 用户列表
GET /users:name - 获取用户详情（管理员）
策略管理 /policies（需要认证）
POST /policies - 创建策略
DELETE /policies - 批量删除策略
DELETE /policies:name - 删除策略
PUT /policies:name - 更新策略
GET /policies - 策略列表
GET /policies:name - 获取策略详情
密钥管理 /secrets（需要认证）
POST /secrets - 创建密钥
DELETE /secrets:name - 删除密钥
PUT /secrets:name - 更新密钥
GET /secrets - 密钥列表
GET /secrets:name - 获取密钥详情
4.安全特性
JWT认证: 使用JWT策略进行用户认证
自动认证: 通过autoAuth()中间件保护路由
输入验证: 使用自定义Gin验证器
权限控制: 区分管理员API和普通用户API
5.设计特点
模块化路由: 按资源类型分组管理路由
中间件链: 使用认证、验证、发布等中间件
工厂模式: 通过mysql.GetMySQLFactoryOr()获取存储实例
错误处理: 统一使用核心响应写入器处理错误
这个包定义了IAM系统的完整RESTful API接口，提供了用户、策略和密钥的完整CRUD操作能力。

核心概念比喻
首先，我们把整个系统想象成一个高级会员制餐厅：
API 接口 (如 /v1/users)：就是餐厅里不同的包厢或区域。
客户端请求 (Client Request)：就是想要进入包厢的客人。
身份验证 (Authentication)：就是餐厅入口的保安，负责检查客人是否有资格进入。
Token：就是保安发给客人的会员卡或一次性手环。
这个餐厅有两种入场方式（两种验证策略）：
Basic 方式 (BasicStrategy)：客人直接报上手机号和密码 (username:password)。这就像告诉保安你的预约信息。简单，但不安全，每次都要报密码。
JWT 方式 (JWTStrategy)：客人出示一张之前已经办理好的会员卡（JWT Token）。保安刷卡验证卡片有效性。安全且方便，一次登录，多处使用。
现在的问题是：保安怎么知道客人用的是哪种方式呢？这就是 AutoStrategy（自动策略）要解决的问题。
----------------------------
自动验证 (AutoStrategy) 的工作流程
AutoStrategy 就像一个聪明的保安总管，他的工作流程非常简单：
拦截客人 (Intercept)：每个客人（请求）在进入餐厅（访问API）前，都会被保安总管拦下。
查看凭证格式 (Check Credential Format)：总管会看一眼客人提供的凭证（Authorization HTTP 请求头）。
如果凭证以 "Basic " 开头（例如：Authorization: Basic dXNlcjpwYXNz），总管就明白了：“哦，这位客人想用账号密码登录。” 他立刻叫来专门处理账号密码的 Basic保安 (a.basic) 来接待这位客人。
如果凭证以 "Bearer " 开头（例如：Authorization: Bearer eyJhbGciOiJ...），总管就明白了：“这位客人有会员卡。” 他立刻叫来专门刷会员卡的 JWT保安 (a.jwt) 来接待。
交给专业保安处理 (Delegate)：保安总管自己不处理具体验证，他只是根据凭证的类型，自动选择并派遣对应的专业保安（策略）来完成真正的验证工作。
处理意外 (Handle Errors)：如果客人提供的凭证格式不对（既不是Basic也不是Bearer，或者格式错误），总管会直接拒绝客人进入，并告诉他：“你的凭证格式不对。
//创建保安总管,有两个保安basic和jwt
// 告诉总管：所有找不到的路由（NoRoute），都由你来负责安检
// 对于 /v1 这个区域的所有包厢...
// ... /users 这个包厢不需要安检，可以直接访问（比如登录接口本身就不能要求已登录）
// !!! 重要：告诉总管，/v1 区域下的所有其他包厢，都必须经过安检！
*/
package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/control/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
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
	jwtStrategy, err := newJWTAuth()
	if err != nil {
		return err
	}
	jwt, ok := jwtStrategy.(auth.JWTStrategy)
	if !ok {
		return fmt.Errorf("转换jwtStrategy错误")
	}
	g.Handle(http.MethodPost, "/login", createAutHandler(jwt.LoginHandler))

	g.Handle(http.MethodDelete, "/logout", createAutHandler(jwt.LogoutHandler))

	g.Handle(http.MethodPost, "/refresh", createAutHandler(jwt.RefreshHandler))

	return nil
}

func (g *GenericAPIServer) installApiRoutes() error {
	auto, err := newAutoAuth()
	if err != nil {
		return err
	}
	g.NoRoute(func(c *gin.Context) {
		// // 1. 校验请求方法必须为POST
		// if c.Request.Method != http.MethodPost {
		// 	c.Header("Allow", http.MethodPost)
		// 	core.WriteResponse(
		// 		c,
		// 		errors.WithCode(code.ErrMethodNotAllowed, "仅支持POST方法"),
		// 		nil,
		// 	)
		// 	return
		// }

		// 4. 若不存在允许的方法（说明是“路径不存在”），返回 404
		core.WriteResponse(
			c,
			errors.WithCode(code.ErrPageNotFound, "业务不存在"),
			nil,
		)

	})
	storeIns, _ := mysql.GetMySQLFactoryOr(nil)
	v1 := g.Group("/v1")
	{
		userv1 := v1.Group("/users")
		{
			userController := user.NewUserController(storeIns, g.redis, g.options)
			userv1.Use(auto.AuthFunc(), middleware.Validation())
			userv1.DELETE(":name", userController.Delete)
			userv1.DELETE(":name/force", userController.ForceDelete)
			userv1.POST("", userController.Create)
			userv1.GET(":name", userController.Get)
			userv1.GET("", userController.List)
		}

	}
	return nil
}

func createAutHandler(jwtHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		contentType := c.ContentType()
		if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
			// 表单格式会触发此逻辑，返回 415 + 100007
			core.WriteResponse(c, errors.WithCode(code.ErrUnsupportedMediaType, "不支持的Content-Type..."), nil)
			c.Abort()
			return
		}
		jwtHandler(c) // 格式正确才进入实际登录逻辑
	}
}
