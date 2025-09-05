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
package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/core"
	middleware "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business/auth"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	_ "github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
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
	Username string `form:"username" json:"username" binding:"required"` // 仅校验非空
	Password string `form:"password" json:"password" binding:"required"` // 仅校验非空
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

func newJWTAuth() (middleware.AuthStrategy, error) {
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

	ginjwt, err := jwt.New(&jwt.GinJWTMiddleware{
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

			// 1. 优先从 jwt.IdentityKey 提取（与 payload 对应）
			username, ok := claims[jwt.IdentityKey].(string)

			// 2. 若失败，从 sub 字段提取（payload 中同步存储了该字段）
			if !ok || username == "" {
				username, ok = claims["sub"].(string)
				if !ok || username == "" {

					return nil
				}
			}

			// 3. 后续：设置到 AuthOperator 和上下文（保持之前的逻辑）
			operatorVal, exists := c.Get("AuthOperator")
			if exists {
				if operator, ok := operatorVal.(*middleware.AuthOperator); ok {
					operator.SetUsername(username)
				}
			}

			c.Set(common.UsernameKey, username)
			ctx := context.WithValue(c.Request.Context(), common.KeyUsername, username)
			c.Request = c.Request.WithContext(ctx)

			return username
		},
		Authorizator:    authorizator(),
		LoginResponse:   loginResponse(),
		RefreshResponse: refreshResponse(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, nil)
		},
		Unauthorized: func(c *gin.Context, httpCode int, message string) {
			var bizCode int
			if len(c.Errors) > 0 {
				// 获取最后一个错误（通常是 Authenticator 返回的自定义错误）
				rawErr := c.Errors.Last().Err
				// 断言为你的自定义错误类型（仅需实现 Code() 方法）
				if customErr, ok := rawErr.(interface{ Code() int }); ok {
					// 直接使用原始错误的业务码（无需依赖 message）
					bizCode = customErr.Code()
					// 新增日志：确认是否提取到正确的 bizCode（如 code.ErrValidation=100004）
					log.Errorf("提取到自定义错误，bizCode=%d", bizCode)

				}
			}
			if bizCode == 0 {
				switch {
				// 匹配Token过期（兼容gin-jwt可能返回的多种过期消息格式）
				case strings.Contains(strings.ToLower(message), "token is expired"):
					bizCode = code.ErrExpired // 100203（Token过期）

				// 匹配签名无效（包括篡改、密钥不匹配等场景）
				case strings.Contains(message, "signature is invalid"):
					bizCode = code.ErrTokenInvalid // 100208（Token无效）

				// 匹配Base64解码失败（如非法字符）
				case strings.HasPrefix(message, "illegal base64 data"):
					bizCode = code.ErrTokenInvalid // 100208（Token无效）

				// 匹配缺少Authorization头
				case message == "Authorization header is not present":
					bizCode = code.ErrMissingHeader // 100205（缺少授权头）

				// 匹配授权头格式错误（如无Bearer前缀）
				case message == "invalid authorization header format":
					bizCode = code.ErrInvalidAuthHeader // 100204（授权头格式无效）

				// 其他未明确匹配的认证错误
				default:
					bizCode = code.ErrUnauthorized // 110003（未授权）
					log.Errorf("进入默认分支，bizCode=%d", bizCode)
				}
			}
			// c.JSON(httpCode, gin.H{
			// 	"code":    bizCode,
			// 	"message": message,
			// 	"data":    nil,
			// })
			err := errors.WithCode(bizCode, message) // 关键修改：移除 "%s" 格式化
			// 新增日志：确认错误的 HTTP 状态码
			if errors.IsWithCode(err) {
				log.Errorf("错误对应的 HTTP 状态码: %d", errors.GetHTTPStatus(err))
			}

			core.WriteResponse(c, err, nil)
			c.Abort()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("建立 JWT middleware 失败: %w", err)
	}

	return auth.NewJWTStrategy(*ginjwt), nil
}

// authoricator 认证逻辑：返回用户信息或具体错误
func authoricator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login loginInfo
		var err error

		// 1. 解析认证信息（Header/Body）：透传解析错误（已携带正确错误码）
		if authHeader := c.Request.Header.Get("Authorization"); authHeader != "" {
			login, err = parseWithHeader(c) // 之前已修复：返回 Basic 认证相关错误码（如 ErrInvalidAuthHeader）
		} else {
			login, err = parseWithBody(c) // 同理：返回 Body 解析相关错误码（如 ErrInvalidParameter）
		}
		if err != nil {
			log.Errorf("parse authentication info failed: %v", err)
			// 关键：直接透传解析错误（无需包装，parse 函数已返回带正确码和提示的 err）
			return nil, err
		}

		if errs := validation.IsQualifiedName(login.Username); len(errs) > 0 {
			errsMsg := strings.Join(errs, ":")
			log.Warnw("用户名不合法:", errsMsg)

			// 直接向上下文写入422响应，不返回错误给框架
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    code.ErrValidation,
				"message": errsMsg,
				"data":    nil,
			})
			c.AbortWithStatus(http.StatusUnprocessableEntity) // 强制终止，不进入框架错误处理
			return nil, nil                                   // 返回nil表示“无错误”，避免框架二次处理
		}
		if err := validation.IsValidPassword(login.Password); err != nil {
			errMsg := "密码不合法：" + err.Error()
			log.Warnw("密码格式不符合要求", err.Error())
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    code.ErrValidation,
				"message": errMsg,
				"data":    nil,
			})
			c.AbortWithStatus(http.StatusUnprocessableEntity)
			return nil, nil
		}

		// 2. 查询用户信息：透传 store 层错误（store 已按场景返回对应码）
		user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: username=%s, error=%v", login.Username, err)
			// 关键：直接透传 store 错误（store 层已返回 ErrUserNotFound/ErrDatabaseTimeout/ErrDatabase）
			return nil, err
		}

		// 3. 密码校验：新增“密码不匹配”场景的错误码（语义匹配）
		if err := user.Compare(login.Password); err != nil {
			log.Errorf("password compare failed: username=%s", login.Username)
			// 场景：密码不正确 → 用通用授权错误码 ErrPasswordIncorrect（100206，401）
			return nil, errors.WithCode(
				code.ErrPasswordIncorrect,
				"密码校验失败：用户名【%s】的密码不正确",
				login.Username,
			)
		}

		// 4. 更新登录时间：忽略非关键错误（仅日志记录，不阻断认证）
		user.LoginedAt = time.Now()
		if updateErr := store.Client().Users().Update(c, user, metav1.UpdateOptions{}); updateErr != nil {
			log.Warnf("update user logined time failed: username=%s, error=%v", login.Username, updateErr)
		}

		return user, nil
	}
}

//go:noinline  // 告诉编译器不要内联此函数
func parseWithHeader(c *gin.Context) (loginInfo, error) {
	// 1. 获取Authorization头
	authHeader := c.Request.Header.Get("Authorization")
	if authHeader == "" {
		// 场景1：授权头为空 → 用通用“缺少授权头”错误码
		return loginInfo{}, errors.WithCode(
			code.ErrMissingHeader,
			"Basic认证：缺少Authorization头，正确格式：Authorization: Basic {base64(username:password)}",
		)
	}

	// 2. 分割前缀和内容（必须为"Basic " + 内容）
	authParts := strings.SplitN(authHeader, " ", 2)
	if len(authParts) != 2 || strings.TrimSpace(authParts[0]) != "Basic" {
		// 场景2：非Basic前缀或分割后长度不对 → 用“授权头格式无效”错误码
		return loginInfo{}, errors.WithCode(
			code.ErrInvalidAuthHeader,
			"Basic认证：授权头格式无效，正确格式：Authorization: Basic {base64(username:password)}（前缀必须为Basic）",
		)
	}
	authPayload := strings.TrimSpace(authParts[1])
	if authPayload == "" {
		// 场景3：Basic前缀后无内容 → 单独判断，提示更精准
		return loginInfo{}, errors.WithCode(
			code.ErrInvalidAuthHeader,
			"Basic认证：Authorization头中Basic前缀后无内容，请提供base64编码的username:password",
		)
	}

	// 3. Base64解码（优先处理解码错误，不掩盖细节）
	payload, decodeErr := base64.StdEncoding.DecodeString(authPayload)
	if decodeErr != nil {
		// 场景4：Base64解码失败 → 用“Base64解码失败”错误码
		return loginInfo{}, errors.WithCode(
			code.ErrBase64DecodeFail,
			"Basic认证：Base64解码失败（%v），请确保内容是username:password的Base64编码",
			decodeErr,
		)
	}

	// 4. 分割用户名和密码（必须含冒号）
	userPassPair := strings.SplitN(string(payload), ":", 2)
	if len(userPassPair) != 2 || strings.TrimSpace(userPassPair[0]) == "" || strings.TrimSpace(userPassPair[1]) == "" {
		// 场景5：解码后无冒号/用户名/密码为空 → 用“payload格式无效”错误码
		return loginInfo{}, errors.WithCode(
			code.ErrInvalidBasicPayload,
			"Basic认证：解码后的内容格式无效，需用冒号分隔非空用户名和密码（如 username:password）",
		)
	}

	// 解析成功，返回用户名密码（去除首尾空格）
	return loginInfo{
		Username: strings.TrimSpace(userPassPair[0]),
		Password: strings.TrimSpace(userPassPair[1]),
	}, nil
}

func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	// 关键：使用 ShouldBindJSON 解析JSON格式的请求体（与测试用例的Content-Type: application/json匹配）
	if err := c.ShouldBindJSON(&login); err != nil {
		// 解析失败时，返回参数错误码（如100004或100006，而非100210）
		return loginInfo{}, errors.WithCode(
			code.ErrInvalidParameter, // 参数错误码（对应400或422，非401）
			"Body参数解析失败：请检查JSON格式是否正确，包含username和password字段",
		)
	}

	// 检查用户名/密码是否为空（基础校验）
	if login.Username == "" || login.Password == "" {
		return loginInfo{}, errors.WithCode(
			code.ErrInvalidParameter,
			"Body参数错误：username和password不能为空",
		)
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
		// user, ok := data.(*v1.User)
		// if !ok {
		// 	log.L(c).Info("无效的user data")
		// 	return false
		// }
		// if user.Status != 1 {
		// 	log.L(c).Warnf("用户%s没有激活", user.Name)
		// 	return false
		// }
		// log.L(c).Infof("用户 `%s`已经通过认证", user.Name) // 添加参数
		// c.Set(common.UsernameKey, user.Name)
		return true
	}
}

func loginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		// 加日志：记录当前响应函数被调用
		core.WriteResponse(c, nil, map[string]string{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
		// c.JSON(http.StatusOK, gin.H{
		// 	"token":  token,
		// 	"expire": expire.Format(time.RFC3339),
		// })
	}
}

func refreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		core.WriteResponse(c, nil, map[string]string{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

// newAutoAuth 创建Auto认证策略（处理所有错误场景，避免panic）
func newAutoAuth() (middleware.AuthStrategy, error) {
	// 1. 初始化JWT认证策略：处理初始化失败错误
	jwtStrategy, err := newJWTAuth()
	if err != nil {
		// 场景1：JWT策略初始化失败（如密钥加载失败、配置错误）
		return nil, errors.WithCode(
			code.ErrInternalServer,
			"Auto认证策略初始化失败：JWT认证策略创建失败，原因：%v",
			err, // 携带原始错误原因，便于调试
		)
	}

	// 2. JWT策略类型转换：处理转换失败（避免强制断言panic）
	jwtAuth, ok := jwtStrategy.(auth.JWTStrategy)
	if !ok {
		// 场景2：类型转换失败（明确预期类型和实际类型）
		return nil, errors.WithCode(
			code.ErrInternalServer,
			"Auto认证策略初始化失败：JWT策略类型转换错误，预期类型为 auth.JWTStrategy，实际类型为 %T",
			jwtStrategy, // 打印实际类型，快速定位依赖问题
		)
	}

	// 3. 初始化Basic认证策略：补充错误处理（原代码直接断言，会panic）
	basicStrategy := newBasicAuth()
	// 3.1 Basic策略类型转换：处理转换失败
	basicAuth, ok := basicStrategy.(auth.BasicStrategy)
	if !ok {
		// 场景3：Basic策略类型转换失败
		return nil, errors.WithCode(
			code.ErrInternalServer,
			"Auto认证策略初始化失败：Basic策略类型转换错误，预期类型为 auth.BasicStrategy，实际类型为 %T",
			basicStrategy,
		)
	}

	// 4. 所有依赖初始化成功，创建AutoStrategy
	autoAuth := auth.NewAutoStrategy(basicAuth, jwtAuth)
	return autoAuth, nil
}
