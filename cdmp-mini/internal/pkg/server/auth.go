package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt/v4"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/core"
	middleware "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/business/auth"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	_ "github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/validator/jwtvalidator"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/idutil"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"

	"github.com/spf13/viper"
)

// Redis键名常量（统一前缀避免冲突）
const (
	redisRefreshTokenPrefix = "auth:refresh_token:"
	redisLoginFailPrefix    = "auth:login_fail:"
	redisBlacklistPrefix    = "auth:blacklist:"
	redisUserSessionsPrefix = "auth:user_sessions:"
)

// 登录失败限制配置
const (
	maxLoginFails   = 5
	loginFailExpire = 15 * time.Minute
)

// 系统常量
const (
	// APIServerAudience defines the value of jwt audience field.
	APIServerAudience = "https://github.com/maxiaolu1981/cretem"

	// Issuer - 标识令牌的"签发系统"（系统视角）
	APIServerIssuer = "https://github.com/maxiaolu1981/cretem"
	// Realm - 标识受保护的"资源领域"（用户视角）
	APIServerRealm = "github.com/maxiaolu1981/cretem"
)

type loginInfo struct {
	Username string `form:"username" json:"username" ` // 仅校验非空
	Password string `form:"password" json:"password" ` // 仅校验非空
}

// 认证策略工厂
func newBasicAuth() middleware.AuthStrategy {
	return auth.NewBasicStrategy(func(username string, password string) bool {
		// fetch user from database
		user, err := interfaces.Client().Users().Get(context.TODO(), username, metav1.GetOptions{})
		if err != nil {
			return false
		}

		// Compare the login password with the user password.
		if err := user.Compare(password); err != nil {
			return false
		}

		user.LoginedAt = time.Now()
		_ = interfaces.Client().Users().Update(context.TODO(), user, metav1.UpdateOptions{})

		return true
	})
}

func newJWTAuth(g *GenericAPIServer) (middleware.AuthStrategy, error) {
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

		Authenticator: func(c *gin.Context) (interface{}, error) {
			return g.authenticate(c)
		},
		PayloadFunc: func(data interface{}) jwt.MapClaims {
			return g.payload(data)
		},
		IdentityHandler: func(c *gin.Context) interface{} {
			originalUserVal, originalExists := c.Get("username")
			if originalExists && originalUserVal != nil {
				if originalUser, ok := originalUserVal.(*v1.User); ok {
					// 验证原始用户的核心字段非空（避免空结构体）
					if originalUser.Name != "" && originalUser.InstanceID != "" {
						// 检查黑名单（原有逻辑保留）
						claims := jwt.ExtractClaims(c)
						jti, ok := claims["jti"].(string)
						if ok && jti != "" {
							isBlacklisted, err := isTokenInBlacklist(g, c, jti)
							if err != nil {
								log.Errorf("IdentityHandler: 检查黑名单失败，error=%v", err)
								return nil
							}
							if isBlacklisted {
								log.Warnf("IdentityHandler: 令牌在黑名单，jti=%s", jti)
								return nil
							}
						}

						// 复用原始 *v1.User，不覆盖为空
						log.Debugf("IdentityHandler: 复用原始 user，username=%s", originalUser.Name)
						c.Set("username", originalUser) // 显式确认存储类型
						return originalUser
					}
				}
			}

			claims := jwt.ExtractClaims(c)
			//优先从 jwt.IdentityKey 提取（与 payload 对应）
			username, ok := claims[jwt.IdentityKey].(string)

			//若失败，从 sub 字段提取（payload 中同步存储了该字段）
			if !ok || username == "" {
				username, ok = claims["sub"].(string)
				if !ok || username == "" {
					return nil
				}
			}
			// 检查令牌是否在黑名单中（带Redis容错）
			jti, ok := claims["jti"].(string)
			if !ok {
				log.Warn("从claims获取jti失败")
			}

			if ok && jti != "" {
				isBlacklisted, err := isTokenInBlacklist(g, c, jti)
				if err != nil {
					return errors.New("安全服务不可用,拒绝服务")
				}
				if isBlacklisted {
					log.Warnf("令牌以已经被注销,jti=%s", jti)
					return nil
				}
			}

			//后续：设置到 AuthOperator 和上下文（保持之前的逻辑）
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
		Authorizator:          authorizator(),
		HTTPStatusMessageFunc: errors.HTTPStatusMessageFunc,
		LoginResponse: func(c *gin.Context, statusCode int, token string, expire time.Time) {
			g.loginResponse(c, statusCode, token, expire)
		},
		RefreshResponse: refreshResponse(),
		LogoutResponse: func(c *gin.Context, codep int) {
			g.logoutRespons(c, codep)
		},
		Unauthorized: handleUnauthorized,
	})
	if err != nil {
		return nil, fmt.Errorf("建立 JWT middleware 失败: %w", err)
	}
	return auth.NewJWTStrategy(*ginjwt), nil
}

// authoricator 认证逻辑：返回用户信息或具体错误
func (g *GenericAPIServer) authenticate(c *gin.Context) (interface{}, error) {
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
		recordErrorToContext(c, err)
		return nil, err
	}
	//检查登录异常
	failCount, err := g.getLoginFailCount(c, login.Username)
	if err != nil {
		log.Debugf("%v", err)
	}
	if failCount > maxLoginFails {
		err := errors.WithCode(code.ErrPasswordIncorrect, "登录失败次数太多,15分钟后重试")
		recordErrorToContext(c, err)
		return nil, err
	}

	if errs := validation.IsQualifiedName(login.Username); len(errs) > 0 {
		errsMsg := strings.Join(errs, ":")
		log.Warnw("用户名不合法:", errsMsg)
		err := errors.WithCode(code.ErrValidation, "%s", errsMsg)
		recordErrorToContext(c, err)
		return nil, err

	}
	if err := validation.IsValidPassword(login.Password); err != nil {
		errMsg := "密码不合法：" + err.Error()
		err := errors.WithCode(code.ErrValidation, "%s", errMsg)
		recordErrorToContext(c, err)
		return nil, err
	}

	//查询用户信息：透传 store 层错误（store 已按场景返回对应码）
	user, err := interfaces.Client().Users().Get(c, login.Username, metav1.GetOptions{})
	if err != nil {
		log.Errorf("get user information failed: username=%s, error=%v", login.Username, err)
		recordErrorToContext(c, err)
		return nil, err
	}

	//密码校验：新增“密码不匹配”场景的错误码（语义匹配）
	if err := user.Compare(login.Password); err != nil {
		log.Errorf("password compare failed: username=%s", login.Username)
		// 场景：密码不正确 → 用通用授权错误码 ErrPasswordIncorrect（100206，401）
		err := errors.WithCode(code.ErrPasswordIncorrect, "密码校验失败：用户名【%s】的密码不正确", login.Username)
		recordErrorToContext(c, err)
		return nil, err
	}
	if err := g.restLoginFailCount(login.Username); err != nil {
		log.Errorf("重置登录次数失败:username=%s,error=%v", login.Username, err)
	}

	//更新登录时间：忽略非关键错误（仅日志记录，不阻断认证）
	user.LoginedAt = time.Now()
	if updateErr := interfaces.Client().Users().Update(c, user, metav1.UpdateOptions{}); updateErr != nil {
		log.Warnf("update user logined time failed: username=%s, error=%v", login.Username, updateErr)
	}
	// 新增：在返回前打印 user 信息，确认非 nil
	// 5. 关键：打印返回前的用户数据，确认有效
	log.Debugf("authenticate: 成功返回用户数据，username=%s，InstanceID=%s，user=%+v",
		user.Name, user.InstanceID, user)
	c.Set("current_user", user)
	return user, nil
}

func (g *GenericAPIServer) logoutRespons(c *gin.Context, codep int) {
	// 获取请求头中的令牌（带Bearer前缀）
	token := c.GetHeader("Authorization")
	claims, err := jwtvalidator.ValidateToken(token, g.options.JwtOptions.Key)
	if err != nil {
		if !errors.IsWithCode(err) {
			// 非预期错误类型，返回默认未授权
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    code.ErrUnauthorized,
				"message": "令牌校验失败",
			})
			return
		}
		bid := errors.GetCode(err)
		// 2.2 无令牌或令牌过期，友好返回已登出
		if bid == code.ErrMissingHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    code.ErrMissingHeader,
				"message": "请先登录",
			})
			return
		}
		if bid == code.ErrExpired {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    code.ErrExpired,
				"message": "令牌已经过期,请重新登录",
			})
			return
		}
		message := errors.GetMessage(err)
		// 2.3 其他错误（如签名无效）返回具体业务码
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    bid,
			"message": message,
		})
		return
	}

	//处理黑名单
	expTimestamp := claims.ExpiresAt.Time
	jti := claims.ID
	if err := g.addTokenToBlacklist(jti, expTimestamp); err != nil {
		log.Errorf("将令牌加入黑名单失败: jti=%s, error=%v", jti, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    code.ErrInternal,
			"message": "将令牌加入黑名单失败",
		})
		return
	}

	//删除刷新令牌（从框架上下文的"refresh_token"键获取，框架内置键名）
	refreshToken, rtExists := c.Get("refresh_token")
	if !rtExists {
		log.Warnf("上下文未找到refresh_token，跳过删除刷新令牌步骤")
	} else {
		rtStr, rtOk := refreshToken.(string)
		if !rtOk {
			log.Warnf("refresh_token类型错误，跳过删除刷新令牌步骤")
		} else if err := g.deleteRefreshToken(rtStr); err != nil {
			log.Warnf("删除刷新令牌失败:error=%v", err)
		}
	}
	//  从用户会话集合中移除刷新令牌
	if refreshToken, rtExists := c.Get("refresh_token"); rtExists {
		rtStr, rtOk := refreshToken.(string)
		if !rtOk {
			log.Warnf("refresh_token类型错误，跳过从用户会话移除步骤")
		} else if err := g.removeRefreshTokenFromUserSessions(claims.UserID, rtStr); err != nil {
			log.Warnf("从用户会话移除刷新令牌失败: user_id=%s, error=%v", claims.UserID, err)
		}
	}

	// 4. 登出成功响应
	log.Infof("登出成功，user_id=%s", claims.UserID)
	// 🔧 优化4：成功场景也通过core.WriteResponse，确保格式统一（code=成功码，message=成功消息）
	core.WriteResponse(c, nil, "登出成功")
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
			code.ErrValidation,
			"Body参数错误：username和password不能为空",
		)
	}

	return login, nil
}

func (g *GenericAPIServer) payload(data interface{}) jwt.MapClaims {

	expirationTime := time.Now().Add(g.options.JwtOptions.Timeout)
	claims := jwt.MapClaims{
		"iss": APIServerIssuer,
		"aud": APIServerAudience,
		"iat": time.Now().Unix(),
		"jti": idutil.GetUUID36("jwt_"),
		"exp": expirationTime.Unix(),
	}
	if u, ok := data.(*v1.User); ok {
		claims["username"] = u.Name
		claims["sub"] = u.Name
		//先写死，后面再调整
		claims["role"] = "admin"
		claims["user_id"] = u.ID

	}
	return claims
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
		path := c.Request.URL.Path
		if strings.HasPrefix(user.Name, "/admin/") && user.Role != "admin" {
			log.L(c).Warnf("用户%s无权访问%s(需要管理员校色)", user.Name, path)
			return false
		}
		log.L(c).Infof("用户 `%s`已经通过认证", user.Name) // 添加参数
		c.Set(common.UsernameKey, user.Name)
		// 新增：在返回前打印 user 信息，确认非 nil
		log.Infof("authenticate 函数即将返回 user：username=%s, user_id=%s, 类型=%T", user.Name, user.InstanceID, user)

		return true
	}
}

func (g *GenericAPIServer) loginResponse(c *gin.Context, statusCode int, token string, expire time.Time) {
	// 从上下文中获取用户信息
	userVal, exists := c.Get("current_user")
	if !exists {
		log.Errorf("loginResponse: 上下文未找到用户数据（键：%s）", jwt.IdentityKey)
		return
	}
	// 类型断言为 *v1.User（与 Authenticator 返回的类型一致）
	user, ok := userVal.(*v1.User)
	if !ok {
		log.Errorf("loginResponse: 用户数据类型错误，预期 *v1.User，实际 %T", userVal)
		return
	}

	// 1. 手动生成刷新令牌
	refreshToken, err := g.generateRefreshToken(user)
	if err != nil {
		log.Errorf("生成刷新令牌失败: %v", err)
		core.WriteResponse(c, errors.WithCode(code.ErrTokenInvalid, "生成刷新令牌失败"), nil)
		return
	}

	//存储到redis中
	if err := g.storeRefreshToken(user.InstanceID, refreshToken, g.options.JwtOptions.MaxRefresh); err != nil {
		log.Warnf("存储刷新令牌失败:error=%v", err)
	}
	//加日志：记录当前响应函数被调用
	core.WriteResponse(c, nil, map[string]string{
		"access_token":  token,
		"refresh_token": refreshToken,
		"expire":        expire.Format(time.RFC3339),
		"token_type":    "Bearer",
	})
}

func refreshResponse() func(c *gin.Context, codef int, token string, expire time.Time) {
	return func(c *gin.Context, codef int, token string, expire time.Time) {

		refresh_token, ok := c.Get("refresh_token")
		if !ok {
			log.Warn("refresh_token获取失败")
		}
		rtStr := refresh_token.(string)
		if !ok {
			log.Warn("刷新令牌获取错误")
			core.WriteResponse(c, errors.WithCode(code.ErrInvalidParameter, "刷新令牌获取错误"), nil)
			return
		}
		core.WriteResponse(c, nil, map[string]string{
			"access_token":  token,
			"refresh_token": string(rtStr),
			"expire_in":     expire.Format(time.RFC3339),
			"token_type":    "Bearer",
		})
	}
}

// newAutoAuth 创建Auto认证策略（处理所有错误场景，避免panic）
func newAutoAuth(g *GenericAPIServer) (middleware.AuthStrategy, error) {
	// 1. 初始化JWT认证策略：处理初始化失败错误
	jwtStrategy, err := newJWTAuth(g)
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

func recordErrorToContext(c *gin.Context, err error) {
	if err != nil {
		c.Errors = append(c.Errors, &gin.Error{
			Err:  err,
			Type: gin.ErrorTypePrivate, // 标记为私有错误，避免框架暴露敏感信息
		})
	}
}

// handleUnauthorized 统一处理未授权场景（封装Unauthorized回调核心逻辑）
// 参数：
//   - c: gin上下文（用于获取请求信息、返回响应）
//   - httpCode: HTTPStatusMessageFunc映射后的HTTP状态码
//   - message: HTTPStatusMessageFunc映射后的基础错误消息
func handleUnauthorized(c *gin.Context, httpCode int, message string) {
	// 1. 从上下文提取业务码（优先使用HTTPStatusMessageFunc映射后的withCode错误）
	bizCode := extractBizCode(c, message)

	// 2. 日志分级：基于业务码重要性输出差异化日志（含request-id便于追踪）
	logWithRequestID(c, bizCode, message)

	// 3. 补充上下文信息：不同业务码返回专属指引（帮助客户端快速定位问题）
	extraInfo := buildExtraInfo(c, bizCode)

	// 4. 生成标准withCode错误（避免格式化安全问题）
	err := errors.WithCode(bizCode, "%s", message)

	// 5. 统一返回响应（依赖core.WriteResponse确保格式一致）
	core.WriteResponse(c, err, extraInfo)

	// 6. 终止流程：防止后续中间件覆盖当前响应
	c.Abort()
}

// extractBizCode 提取业务码（优先从c.Errors获取，降级用消息匹配）
func extractBizCode(c *gin.Context, message string) int {
	// 优先：从c.Errors提取带Code()方法的错误（HTTPStatusMessageFunc映射后的结果）
	if len(c.Errors) > 0 {
		rawErr := c.Errors.Last().Err
		log.Debugf("[handleUnauthorized] 从c.Errors获取原始错误: %+v", rawErr)

		// 适配自定义withCode错误（必须实现Code() int方法）
		if customErr, ok := rawErr.(interface{ Code() int }); ok {
			bizCode := customErr.Code()
			log.Infof("[handleUnauthorized] 从错误中提取业务码: %d（request-id: %s）",
				bizCode, getRequestID(c))
			return bizCode
		}
	}

	// 降级：若无法直接提取，基于消息文本匹配业务码（覆盖所有授权认证相关业务码）
	msgLower := strings.ToLower(message)
	switch {
	case strings.Contains(msgLower, "expired"):
		return code.ErrExpired // 100203：令牌已过期
	case strings.Contains(msgLower, "signature") && strings.Contains(msgLower, "invalid"):
		return code.ErrSignatureInvalid // 100202：签名无效
	case strings.Contains(msgLower, "authorization") && strings.Contains(msgLower, "not present"):
		return code.ErrMissingHeader // 100205：缺少Authorization头
	case strings.Contains(msgLower, "authorization") && strings.Contains(msgLower, "invalid format"):
		return code.ErrInvalidAuthHeader // 100204：授权头格式无效
	case strings.Contains(msgLower, "base64") && strings.Contains(msgLower, "decode"):
		return code.ErrBase64DecodeFail // 100209：Basic认证Base64解码失败
	case strings.Contains(msgLower, "basic") && strings.Contains(msgLower, "payload"):
		return code.ErrInvalidBasicPayload // 100210：Basic认证payload格式无效
	case strings.Contains(msgLower, "invalid") && (strings.Contains(msgLower, "token") || strings.Contains(msgLower, "jwt")):
		return code.ErrTokenInvalid // 100208：令牌无效
	case strings.Contains(msgLower, "password") && strings.Contains(msgLower, "incorrect"):
		return code.ErrPasswordIncorrect // 100206：密码不正确
	case strings.Contains(msgLower, "permission") && strings.Contains(msgLower, "denied"):
		return code.ErrPermissionDenied // 100207：权限不足
	default:
		log.Warnf("[handleUnauthorized] 未匹配到业务码，使用默认未授权码（request-id: %s），原始消息: %s",
			getRequestID(c), message)
		return code.ErrUnauthorized // 110003：默认未授权
	}
}

// logWithRequestID 带request-id的分级日志（按业务码重要性划分级别）
func logWithRequestID(c *gin.Context, bizCode int, message string) {
	requestID := getRequestID(c)
	switch bizCode {
	// 安全风险：Warn级别（需重点关注，可能是恶意请求）
	case code.ErrSignatureInvalid, code.ErrTokenInvalid, code.ErrPasswordIncorrect:
		log.Warnf("[安全风险] 未授权（bizCode: %d），request-id: %s，消息: %s",
			bizCode, requestID, message)
	// 客户端错误：Debug级别（便于客户端调试，非恶意）
	case code.ErrInvalidAuthHeader, code.ErrBase64DecodeFail, code.ErrInvalidBasicPayload:
		log.Debugf("[客户端错误] 未授权（bizCode: %d），request-id: %s，消息: %s",
			bizCode, requestID, message)
	// 常规场景：Info级别（正常用户操作，如令牌过期、缺少头）
	case code.ErrExpired, code.ErrMissingHeader, code.ErrPermissionDenied:
		log.Infof("[常规场景] 未授权（bizCode: %d），request-id: %s，消息: %s",
			bizCode, requestID, message)
	// 未分类：Warn级别（需后续补充匹配规则）
	default:
		log.Warnf("[未分类] 未授权（bizCode: %d），request-id: %s，消息: %s",
			bizCode, requestID, message)
	}
}

// buildExtraInfo 基于业务码构建额外上下文信息（帮助客户端快速修复问题）
// 关键：给函数增加 c *gin.Context 参数，用于获取请求头/上下文信息
func buildExtraInfo(c *gin.Context, bizCode int) gin.H {
	switch bizCode {
	case code.ErrExpired: // 100203：令牌已过期
		return gin.H{
			"suggestion": "令牌已过期，请重新调用/login接口获取新令牌",
			"next_step":  "POST /login（携带用户名密码）",
		}
	case code.ErrInvalidAuthHeader: // 100204：授权头格式无效
		return gin.H{
			"example": "正确格式：Authorization: Bearer <your-jwt-token>（Bearer后需带1个空格）",
			"note":    "仅支持Bearer认证方案，不支持Basic/其他方案",
		}
	case code.ErrTokenInvalid: // 100208：令牌无效
		return gin.H{
			"possible_reason": []string{
				"令牌格式错误（需包含2个.分隔，如xx.xx.xx）",
				"令牌被篡改（签名验证失败）",
				"令牌未经过正确编码（需Base64Url编码）",
			},
		}
	case code.ErrBase64DecodeFail: // 100209：Basic认证Base64解码失败
		return gin.H{
			"example":    "正确格式：Authorization: Basic dXNlcjE6cGFzc3dvcmQ=（dXNlcjE6cGFzc3dvcmQ=是base64(\"user1:password\")）",
			"check_tool": "可通过echo -n 'user:pass' | base64 验证编码是否正确",
		}
	case code.ErrInvalidBasicPayload: // 100210：Basic认证payload格式无效
		return gin.H{
			"requirement": "Base64解码后必须包含冒号（:），格式为\"用户名:密码\"",
			"example":     "解码后应为\"admin:123456\"，而非\"admin123456\"",
		}
	case code.ErrPermissionDenied: // 100207：权限不足
		// 现在 c 是函数参数，可正常调用 GetHeader 获取 X-User 头
		currentUser := c.GetHeader("X-User")
		// 优化：若 X-User 头为空，返回“未知用户”避免空值
		if currentUser == "" {
			currentUser = "未知用户（未携带X-User头）"
		}
		return gin.H{
			"suggestion":   "联系管理员授予操作权限（需包含xxx角色）",
			"current_user": currentUser, // 正常返回当前用户信息
		}
	default: // 其他场景：返回空（避免冗余）
		return gin.H{}
	}
}

// getRequestID 从上下文获取request-id（便于链路追踪）
func getRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("requestID"); exists {
		if idStr, ok := requestID.(string); ok {
			return idStr
		}
	}
	// 降级：从请求头获取
	return c.GetHeader("X-Request-ID")
}

func isTokenInBlacklist(g *GenericAPIServer, c *gin.Context, jti string) (bool, error) {

	key := g.options.JwtOptions.Blacklist_key_prefix + jti
	exists, err := g.redis.Exists(c, key)
	if err != nil {
		return false, errors.WithCode(code.ErrUnknown, "查询黑名单失效")
	}
	return exists, nil
}

func (g *GenericAPIServer) getLoginFailCount(ctx *gin.Context, username string) (int, error) {
	if !g.checkRedisAlive() {
		log.Warnf("Redis不可用,暂时跳过登录失败次数限制")
		return 0, nil
	}
	key := redisLoginFailPrefix + username
	log.Debugf("key:%s", key)
	val, err := g.redis.GetKey(ctx, key)
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0, nil
		}
		log.Warnf("获取登录失败次数失败:username=%s,error=%v", username, err)
		return 0, nil
	}
	count, err := strconv.Atoi(val)
	if err != nil {
		log.Warnf("解析登录失败次数失败: username=%s, val=%s, error=%v", username, val, err)
		return 0, nil // 解析失败默认返回0次
	}
	return count, nil
}

func (g *GenericAPIServer) checkRedisAlive() bool {
	if err := g.redis.Up(); err != nil { // 用Ping检查存活
		log.Errorf("Redis连接失败: %v", err)
		return false
	}
	return true
}

func (g *GenericAPIServer) restLoginFailCount(username string) error {
	if !g.checkRedisAlive() {
		log.Warnf("Redis不可用,无法重置登录失败次数:username:%s", username)
		return nil
	}
	key := redisLoginFailPrefix + username
	if !g.redis.DeleteKey(context.TODO(), key) {
		return errors.New("重置登录次数失败")
	}
	return nil
}

func (g *GenericAPIServer) storeRefreshToken(userID, refreshToken string, expire time.Duration) error {
	if !g.checkRedisAlive() {
		log.Warn("Redis不可用，无法删除刷新令牌")
	}
	rtKey := redisRefreshTokenPrefix + refreshToken
	if err := g.redis.SetRawKey(
		context.TODO(),
		rtKey,
		userID,
		expire,
	); err != nil {
		log.Warnf("添加刷新令牌到用户会话集合失败: user_id=%s, error=%v", userID, err)
	}

	userSessionsKey := redisUserSessionsPrefix + userID
	if err := g.redis.AddToSet(
		context.TODO(),
		userSessionsKey,
		refreshToken); err != nil {
		log.Warnf("添加刷新令牌到用户会话集合失败: user_id=%s, error=%v", userID, err)

	}

	if err := g.redis.SetExp(
		context.TODO(),
		userSessionsKey,
		expire,
	); err != nil {
		log.Warnf("设置令牌失效时间: %v", err)
		return err
	}

	return nil
}

func (g *GenericAPIServer) deleteRefreshToken(refreshToken string) error {
	if !g.checkRedisAlive() {
		log.Warn("Redis不可用，无法删除刷新令牌")
		return errors.New("无法删除刷新令牌")
	}

	rtKey := redisRefreshTokenPrefix + refreshToken
	if !g.redis.DeleteRawKey(context.TODO(), rtKey) {
		return errors.New("删除刷新令牌错误")
	}
	return nil
}

func (g *GenericAPIServer) removeRefreshTokenFromUserSessions(userID, refreshToken string) error {
	if !g.checkRedisAlive() {
		log.Warnf("Redis不可用，无法从用户会话移除刷新令牌: user_id=%s", userID)
		return nil
	}

	userSessionsKey := redisUserSessionsPrefix + userID
	if err := g.redis.RemoveFromSet(
		context.TODO(),
		userSessionsKey,
		refreshToken,
	); err != nil {
		return fmt.Errorf("remove from user sessions failed: %w", err)
	}
	return nil
}

func (g *GenericAPIServer) addTokenToBlacklist(jti string, expireAt time.Time) error {
	if !g.checkRedisAlive() {
		return errors.WithCode(code.ErrInternal, "系统缓存不可用，无法注销令牌")
	}
	key := g.options.JwtOptions.Blacklist_key_prefix + jti
	log.Debugf("黑名单key:%s", key)
	expire := expireAt.Sub(time.Now()) + time.Hour
	if expire < 0 {
		expire = time.Hour
	}
	// 使用SetRawKey存储黑名单键值对
	if err := g.redis.SetRawKey(
		context.TODO(),
		key,
		"1",
		expire,
	); err != nil {
		return fmt.Errorf("添加到黑名单失败: %w", err)
	}
	return nil

}

// generateRefreshToken 生成刷新令牌
func (g *GenericAPIServer) generateRefreshToken(user *v1.User) (string, error) {

	// 创建刷新令牌的 claims
	refreshClaims := gojwt.MapClaims{
		"iss":     APIServerIssuer,
		"aud":     APIServerAudience,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(g.options.JwtOptions.MaxRefresh).Unix(),
		"jti":     idutil.GetUUID36("refresh_"),
		"sub":     user.Name,
		"user_id": user.ID,
		"type":    "refresh", // 标记为刷新令牌
	}

	// 生成刷新令牌
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, refreshClaims)

	refreshToken, err := token.SignedString([]byte(g.options.JwtOptions.Key))
	if err != nil {
		return "", fmt.Errorf("签名刷新令牌失败: %w", err)
	}

	return refreshToken, nil
}
