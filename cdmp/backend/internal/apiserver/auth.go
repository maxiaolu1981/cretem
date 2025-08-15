/*
包摘要
该包（package apiserver）实现了 APIServer 的多种认证策略，基于 Gin 框架和 JWT 中间件，提供了 Basic 认证、JWT 认证以及自动切换认证方式的功能。核心作用是处理用户登录验证、生成 / 刷新认证令牌、授权检查等身份认证相关逻辑，确保只有合法用户能访问受保护的资源。
核心流程
该包的核心是通过不同函数创建三种认证策略（Basic、JWT、自动切换），并定义了认证过程中的关键逻辑（如登录信息解析、用户验证、令牌生成等），整体流程如下：
认证策略创建：
newBasicAuth：创建 Basic 认证策略，通过校验请求中的用户名密码（与数据库存储的用户信息比对）完成认证。
newJWTAuth：创建 JWT 认证策略，基于 JWT 中间件实现，支持登录生成令牌、令牌刷新、令牌验证等功能。
newAutoAuth：组合 Basic 和 JWT 策略，实现自动切换认证方式的策略。
认证核心逻辑：
登录信息解析：parseWithHeader 从 Authorization 头解析 Basic 认证信息，parseWithBody 从请求体解析 JSON 格式的登录信息。
用户验证：authenticator 函数统一处理认证逻辑，通过数据库查询用户信息，验证密码正确性，并更新用户最后登录时间。
令牌处理：JWT 策略中，payloadFunc 定义令牌包含的声明信息（如发行者、受众、用户名），loginResponse 和 refreshResponse 定义登录 / 刷新令牌的响应格式。
授权检查：authorizator 函数验证认证通过的用户身份，确认是否允许访问资源。

*/

package apiserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2" // JWT 中间件，用于实现基于令牌的认证
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"     // 数据库操作接口，用于查询/更新用户信息
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware"      // 认证策略接口定义
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/middleware/auth" // 具体的认证策略实现（Basic/JWT）
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"             // 用户相关的 API 定义
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"   // 元数据定义（如查询选项）
	"github.com/spf13/viper"                                                   // 配置管理工具，用于读取 JWT 相关配置

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log" // 日志工具
)

const (
	// APIServerAudience 定义 JWT 令牌中 "aud"（受众）字段的值，标识令牌的目标接收者
	APIServerAudience = "iam.api.marmotedu.com"

	// APIServerIssuer 定义 JWT 令牌中 "iss"（发行者）字段的值，标识令牌的发行来源
	APIServerIssuer = "iam-apiserver"
)

// loginInfo 用于接收客户端的登录信息（用户名和密码）
// 支持 form 表单和 JSON 两种格式，通过 binding 标签做参数校验
type loginInfo struct {
	Username string `form:"username" json:"username" binding:"required,username"` // 用户名，必填
	Password string `form:"password" json:"password" binding:"required,password"` // 密码，必填
}

// newBasicAuth 创建 Basic 认证策略
// 实现 middleware.AuthStrategy 接口，返回 Basic 认证的中间件函数
func newBasicAuth() middleware.AuthStrategy {
	// 基于 auth.BasicStrategy 实现，传入一个验证函数（校验用户名密码）
	return auth.NewBasicStrategy(func(username string, password string) bool {
		// 从数据库查询用户信息
		user, err := store.Client().Users().Get(context.TODO(), username, metav1.GetOptions{})
		if err != nil {
			// 用户不存在或查询失败，返回认证失败
			return false
		}

		// 比对登录密码与用户存储的密码（通常是哈希比对）
		if err := user.Compare(password); err != nil {
			return false
		}

		// 认证成功，更新用户最后登录时间
		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(context.TODO(), user, metav1.UpdateOptions{})

		return true
	})
}

// newJWTAuth 创建 JWT 认证策略
// 在 gin-jwt 中添加自定义认证逻辑，核心是通过配置 jwt.GinJWTMiddleware 的各个回调函数（如 Authenticator、Authorizator 等）来注入业务逻辑。这些回调函数会在认证流程的不同阶段被调用，从而实现完整的 “请求验证 - 令牌生成 - 响应处理” 流程。
func newJWTAuth() middleware.AuthStrategy {
	// 初始化 JWT 中间件配置
	ginjwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            viper.GetString("jwt.Realm"),         // 认证域（用于令牌标识）
		SigningAlgorithm: "HS256",                              // 签名算法
		Key:              []byte(viper.GetString("jwt.key")),   // 签名密钥（从配置读取）
		Timeout:          viper.GetDuration("jwt.timeout"),     // 令牌有效期
		MaxRefresh:       viper.GetDuration("jwt.max-refresh"), // 令牌最长可刷新时间
		Authenticator:    authenticator(),                      // 认证函数（验证用户名密码并返回用户信息）
		LoginResponse:    loginResponse(),                      // 登录成功响应函数
		LogoutResponse: func(c *gin.Context, code int) { // 登出响应函数
			c.JSON(http.StatusOK, nil)
		},
		RefreshResponse: refreshResponse(), // 令牌刷新响应函数
		PayloadFunc:     payloadFunc(),     // 定义令牌中包含的声明信息
		IdentityHandler: func(c *gin.Context) interface{} { // 从令牌中提取身份信息（用户名）
			claims := jwt.ExtractClaims(c)
			return claims[jwt.IdentityKey]
		},
		IdentityKey:  middleware.UsernameKey, // 身份标识的键（用于上下文存储用户名）
		Authorizator: authorizator(),         // 授权检查函数（验证身份是否合法）
		Unauthorized: func(c *gin.Context, code int, message string) { // 未授权响应函数
			c.JSON(code, gin.H{
				"message": message,
			})
		},
		TokenLookup:   "header: Authorization, query: token, cookie: jwt", // 令牌查找位置（请求头、查询参数、Cookie）
		TokenHeadName: "Bearer",                                           // 令牌前缀（如 "Bearer <token>"）
		SendCookie:    true,                                               // 是否将令牌存入 Cookie
		TimeFunc:      time.Now,                                           // 时间函数（用于生成令牌时间戳）
	})

	// 基于初始化的 JWT 中间件创建 JWTStrategy
	return auth.NewJWTStrategy(*ginjwt)
}

// newAutoAuth 创建自动切换的认证策略
// 组合 Basic 和 JWT 策略，可根据请求自动选择合适的认证方式
func newAutoAuth() middleware.AuthStrategy {
	// 将 Basic 和 JWT 策略转换为具体类型后组合
	return auth.NewAutoStrategy(newBasicAuth().(auth.BasicStrategy), newJWTAuth().(auth.JWTStrategy))
}

// authenticator 定义 JWT 认证的核心逻辑：验证用户身份并返回用户信息
func authenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login loginInfo
		var err error

		// 支持从请求头（Basic 认证）或请求体（JSON）获取登录信息
		if c.Request.Header.Get("Authorization") != "" {
			login, err = parseWithHeader(c) // 从 Authorization 头解析
		} else {
			login, err = parseWithBody(c) // 从请求体解析
		}
		if err != nil {
			return "", jwt.ErrFailedAuthentication // 解析失败，返回认证失败
		}

		// 从数据库查询用户信息
		user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: %s", err.Error())
			return "", jwt.ErrFailedAuthentication
		}

		// 比对密码
		if err := user.Compare(login.Password); err != nil {
			return "", jwt.ErrFailedAuthentication
		}

		// 认证成功，更新用户最后登录时间
		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(c, user, metav1.UpdateOptions{})

		return user, nil // 返回用户信息，用于生成 JWT 令牌
	}
}

// parseWithHeader 从 Authorization 头解析 Basic 认证信息
// Basic 认证格式：Authorization: Basic <base64编码的"username:password">
func parseWithHeader(c *gin.Context) (loginInfo, error) {
	// 拆分 Authorization 头（格式：["Basic", "<base64>"]）
	auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		log.Errorf("get basic string from Authorization header failed")
		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	// 解码 base64 字符串（解码后格式："username:password"）
	payload, err := base64.StdEncoding.DecodeString(auth[1])
	if err != nil {
		log.Errorf("decode basic string: %s", err.Error())
		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	// 拆分用户名和密码
	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		log.Errorf("parse payload failed")
		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return loginInfo{
		Username: pair[0],
		Password: pair[1],
	}, nil
}

// parseWithBody 从请求体解析 JSON 格式的登录信息（{username: "...", password: "..."}）
func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	// 绑定并校验请求体（自动处理 JSON 解析，结合 loginInfo 的 binding 标签做参数校验）
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("parse login parameters: %s", err.Error())
		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return login, nil
}

// refreshResponse 定义令牌刷新成功的响应格式
func refreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,                       // 新的令牌
			"expire": expire.Format(time.RFC3339), // 令牌过期时间（RFC3339 格式）
		})
	}
}

// loginResponse 定义登录成功的响应格式（与刷新响应一致，返回令牌和过期时间）
func loginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

// payloadFunc 定义 JWT 令牌中包含的声明信息（Claims）
func payloadFunc() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		claims := jwt.MapClaims{
			"iss": APIServerIssuer,   // 发行者
			"aud": APIServerAudience, // 受众
		}
		// 如果数据是用户对象，添加用户名到声明中
		if u, ok := data.(*v1.User); ok {
			claims[jwt.IdentityKey] = u.Name // 身份标识（用户名）
			claims["sub"] = u.Name           // 主题（通常为用户名）
		}

		return claims
	}
}

// authorizator 定义授权检查逻辑：验证认证通过的用户是否允许访问资源
func authorizator() func(data interface{}, c *gin.Context) bool {
	return func(data interface{}, c *gin.Context) bool {
		// 验证身份信息是否为字符串（用户名）
		if v, ok := data.(string); ok {
			log.L(c).Infof("user `%s` is authenticated.", v)
			return true // 身份合法，允许访问
		}

		return false // 身份非法，拒绝访问
	}
}
