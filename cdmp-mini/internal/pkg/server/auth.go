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
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
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
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"

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
	Username string `form:"username" json:"username" ` // 仅校验非空
	Password string `form:"password" json:"password" ` // 仅校验非空
}

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
		Authorizator:          authorizator(),
		HTTPStatusMessageFunc: errors.HTTPStatusMessageFunc,
		LoginResponse:         loginResponse(),
		RefreshResponse:       refreshResponse(),
		LogoutResponse:        logoutRespons,
		Unauthorized:          handleUnauthorized,
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

		// 2. 查询用户信息：透传 store 层错误（store 已按场景返回对应码）
		user, err := interfaces.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: username=%s, error=%v", login.Username, err)
			recordErrorToContext(c, err)
			return nil, err
		}

		// 3. 密码校验：新增“密码不匹配”场景的错误码（语义匹配）
		if err := user.Compare(login.Password); err != nil {
			log.Errorf("password compare failed: username=%s", login.Username)
			// 场景：密码不正确 → 用通用授权错误码 ErrPasswordIncorrect（100206，401）
			err := errors.WithCode(code.ErrPasswordIncorrect, "密码校验失败：用户名【%s】的密码不正确", login.Username)
			recordErrorToContext(c, err)
			return nil, err

		}

		// 4. 更新登录时间：忽略非关键错误（仅日志记录，不阻断认证）
		user.LoginedAt = time.Now()
		if updateErr := interfaces.Client().Users().Update(c, user, metav1.UpdateOptions{}); updateErr != nil {
			log.Warnf("update user logined time failed: username=%s, error=%v", login.Username, updateErr)
		}

		return user, nil
	}
}

func logoutRespons(c *gin.Context, codep int) {

	// 1. 获取请求头中的令牌（带Bearer前缀）
	rawAuthHeader, exists := c.Get("raw_auth_header")
	if !exists {
		// 降级：若上下文没有，再用 GetHeader（避免极端情况）
		rawAuthHeader = c.GetHeader("Authorization")
		log.Warnf("[logoutRespons] 上下文未找到原始头，降级使用 GetHeader")
	}
	// 转换为字符串（上下文存储的是 interface{} 类型）
	token := rawAuthHeader.(string)
	// 打印日志验证：此时 token 应为 "Bearer "（长度7）
	log.Infof("[logoutRespons] 最终使用的原始令牌：[%q]，长度：%d", token, len(token))
	// 2. 调用validation包的ValidateToken进行校验（已适配withCode错误）
	claims, err := jwtvalidator.ValidateToken(token)
	if err != nil {

		// 🔧 优化2：统一通过core.WriteResponse返回，确保格式一致
		core.WriteResponse(c, err, nil)
		return
	}

	// 3. 令牌有效，执行登出核心逻辑（如加入黑名单）
	if err := destroyToken(claims.UserID); err != nil {
		log.Errorf("登出失败，user_id=%s，err=%v", claims.UserID, err)
		// 🔧 优化3：用WithCode包装错误，再通过统一响应函数返回
		wrappedErr := errors.WithCode(code.ErrInternal, "登出失败，请重试: %v", err)
		core.WriteResponse(c, wrappedErr, nil)
		return
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

func payload() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {

		claims := jwt.MapClaims{
			"iss": APIServerIssuer,
			"aud": APIServerAudience,
		}
		if u, ok := data.(*v1.User); ok {
			claims[jwt.IdentityKey] = u.Name
			claims["sub"] = u.Name
			//先写死，后面再调整
			claims["role"] = "admin"
			claims["user_id"] = u.InstanceID

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
			"access_token": token,
			"expire_in":    expire.Format(time.RFC3339),
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

// maskToken 令牌脱敏（仅保留前6位和后4位，避免日志泄露）
func maskToken(token string) string {
	if len(token) <= 10 {
		return "******"
	}
	return token[:6] + "******" + token[len(token)-4:]
}

// destroyToken 执行登出逻辑（示例：将用户令牌加入黑名单）
// 实际实现需根据你的业务（如Redis黑名单、会话销毁等）
func destroyToken(userID string) error {
	// 示例逻辑：写入Redis黑名单（key=userID，value=过期时间）
	// ctx := context.Background()
	// return redisClient.Set(ctx, "logout:"+userID, time.Now().Unix(), 24*time.Hour).Err()
	return nil
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
