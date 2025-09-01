/*
实现了基于 JWT 和 Basic 认证的双重认证策略，提供完整的身份验证和授权功能。
1. 认证策略工厂
//创建JWT认证策略
// 创建Basic认证策略
// 创建自动选择策略（JWT优先，Basic备用）
2.JWT 认证核心组件
1.) 基础配置：初始化核心参数
realm signingAlgorithm  key timeout maxRefresh identityKey
2.) 令牌解析配置：定义令牌的获取位置和格式
决定框架从哪里读取令牌（请求头/查询参数/ Cookie）
tokenLoopup tokenHeadName sendCookie
3.) 时间函数配置：定义时间获取方式（影响令牌有效期计算） timeFunc
4.) 认证核心函数：用户登录验证逻辑authenticatorFunc
这是认证流程的入口，验证用户凭据（用户名/密码）
5.)载荷生成函数：定义JWT中存储的用户信息 payloadFunc
认证成功后，生成令牌时需要的用户身份信息
6.) 身份提取函数：从JWT中解析用户身份identityHandler
用于后续请求中识别用户（如权限验证）
7.) 权限验证函数：验证用户是否有权限访问资源authorizatorFunc
在身份识别后执行，判断用户是否能访问当前接口
8.)响应处理函数：定义各种场景的响应格式
包括登录成功、刷新令牌、注销、认证失败等
loginResponse()      // 登录成功响应
refreshResponse()    // 令牌刷新响应
logoutResponse()     // 注销响应
unauthorizedFunc()   // 认证失败响应

3. 凭据解析器
// 支持多种认证方式
parseWithHeader()    // 从HTTP Header解析Basic认证
parseWithBody()      // 从请求体解析JSON凭据
*/
// Package apiserver implements the API server handlers.
//nolint:unused // 包含通过闭包间接使用的函数
package apiserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	middleware "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business/auth"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	_ "github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/spf13/viper"
)

const (
	// APIServerAudience defines the value of jwt audience field.
	APIServerAudience = "https://github.com/maxiaolu1981/cretem"

	// Issuer - 标识令牌的"签发系统"（系统视角）
	APIServerIssuer = "https://github.com/maxiaolu1981/cretem"
	// Realm - 标识受保护的"资源领域"（用户视角）
	APIServerRealm = "github.com/maxiaolu1981/cretem"
)

type loginInfo struct {
	Username string `form:"username" json:"username" binding:"required,username"`
	Password string `form:"password" json:"password" binding:"required,password"`
}

func newBasicAuth() middleware.AuthStrategy {
	return auth.NewBasicStrategy(func(username string, password string) bool {
		// fetch user from database
		user, err := store.Client().Users().Get(context.TODO(), username, metav1.GetOptions{})
		if err != nil {
			return false
		}

		// Compare the login password with the user password.
		if err := user.Compare(password); err != nil {
			return false
		}

		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(context.TODO(), user, metav1.UpdateOptions{})

		return true
	})
}

func newJWTAuth() middleware.AuthStrategy {
	//基础配置：初始化核心参数
	realm := viper.GetString("jwt.realm")
	signingAlgorithm := "HS256"
	key := []byte(viper.GetString("jwt.key"))
	timeout := viper.GetDuration("jwt.timeout")
	maxRefresh := viper.GetDuration("jwt.max-refresh")
	identityKey := common.UsernameKey

	// 令牌解析配置：定义令牌的获取位置和格式
	tokenLoopup := "header: Authorization, query: token, cookie: jwt"
	tokenHeadName := "Bearer"

	ginjwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            realm,
		SigningAlgorithm: signingAlgorithm,
		Key:              key,
		Timeout:          timeout,
		MaxRefresh:       maxRefresh,
		IdentityKey:      identityKey,
		TokenLookup:      tokenLoopup,
		TokenHeadName:    tokenHeadName,
		SendCookie:       true,
		TimeFunc:         time.Now,
		Authenticator:    authoricator(),
		PayloadFunc:      payload(),
		IdentityHandler: func(c *gin.Context) interface{} {
			claims := jwt.ExtractClaims(c)
			return claims[jwt.IdentityKey]
		},
		Authorizator:    authorizator(),
		LoginResponse:   loginResponse(),
		RefreshResponse: refreshResponse(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, nil)
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"message": message,
			})
		},
	})
	return auth.NewJWTStrategy(ginjwt)
}

func authoricator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var err error
		var login loginInfo

		if c.Request.Header.Get("Authorization") != "" {
			login, err = parseWithHeader(c)
		} else {
			login, err = parseWithBody(c)
		}
		if err != nil {
			return nil, jwt.ErrFailedAuthentication // 返回nil而不是字符串
		}

		user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: %s", err.Error())
			return nil, jwt.ErrFailedAuthentication
		}

		if err := user.Compare(login.Password); err != nil {
			return nil, jwt.ErrFailedAuthentication
		}

		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(c, user, metav1.UpdateOptions{})
		return user, nil // 统一返回 *v1.User
	}
}

//go:noinline  // 告诉编译器不要内联此函数
func parseWithHeader(c *gin.Context) (loginInfo, error) {
	auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		return loginInfo{}, jwt.ErrFailedAuthentication
	}
	payload, err := base64.StdEncoding.DecodeString(auth[1])
	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 || err != nil {
		return loginInfo{}, jwt.ErrEmptyAuthHeader
	}
	return loginInfo{pair[0], pair[1]}, nil
}

func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	err := c.ShouldBindJSON(&login)
	if err != nil {
		return loginInfo{}, jwt.ErrFailedAuthentication
	}
	return login, nil
}

func payload() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		claims := jwt.MapClaims{
			"iss": APIServerIssuer,
			"aud": APIServerAudience,
		}
		if u, ok := data.(*v1.User); ok {
			claims[jwt.IdentityKey] = u.Name
			claims["sub"] = u.Name
		}
		return claims
	}
}

func authorizator() func(data interface{}, c *gin.Context) bool {
	return func(data interface{}, c *gin.Context) bool {
		user, ok := data.(*v1.User)
		if !ok {
			log.L(c).Info("无效的user data")
			return false
		}
		if user.Status != 1 {
			log.L(c).Warnf("用户%s没有激活", user.Name)
			return false
		}
		log.L(c).Infof("用户 `%s`已经通过认证", user.Name) // 添加参数
		c.Set(common.UsernameKey, user.Name)
		return true
	}
}

func loginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

func refreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}
