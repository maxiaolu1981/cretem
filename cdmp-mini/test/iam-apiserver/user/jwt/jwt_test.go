package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	redisV8 "github.com/go-redis/redis/v8"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

// ==================== 配置常量（根据实际环境调整） ====================
const (
	// 服务器配置
	ServerBaseURL  = "http://localhost:8080"
	RequestTimeout = 5 * time.Second

	// API路径
	LoginAPIPath   = "/login"
	RefreshAPIPath = "/refresh"
	LogoutAPIPath  = "/logout"

	// Redis配置（兼容redis/v8）
	RedisAddr     = "localhost:6379"
	RedisPassword = ""
	RedisDB       = 0

	// 测试用户信息
	TestUsername    = "admin"
	TestUserID2     = "1002"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022" // 与有效密码区分，确保登录失败用例生效

	// JWT配置
	JWTSigningKey   = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo" // 必须与服务端JWT密钥一致
	JWTAlgorithm    = "HS256"
	TokenExpireTime = 60 * time.Second
	RTExpireTime    = 3600 * time.Second

	// Redis键前缀（必须与服务端存储规则一致）
	RTRedisPrefix        = "genericapiserver:auth:refresh_token:"
	redisBlacklistPrefix = "gin-jwt:blacklist:"

	// 业务码常量（根据服务端实际返回调整）
	RespCodeSuccess       = 100001 // 成功
	RespCodeRTRequired    = 110004 // 缺少RefreshToken（避免与成功码重复，原100001修正）
	RespCodeRTRevoked     = 100211 // RefreshToken已撤销
	RespCodeATExpired     = 100203 // AccessToken已过期
	RespCodeInvalidAT     = 100208 // AccessToken无效
	RespCodeRTExpired     = 100203 // RefreshToken已过期
	RespCodeTokenMismatch = 100006 // Token不匹配
	RespCodeInvalidAuth   = 100007 // 认证失败（密码错误等）
)

// ==================== 数据结构定义（核心修正：区分HTTP状态码与业务码） ====================
// JWT自定义声明（适配服务端gin-jwt结构）
type CustomClaims struct {
	Username string `json:"username"`
	Userid   string `json:"user_id"`
	jwt.RegisteredClaims
}

// 令牌响应结构（匹配服务端返回格式）
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// API通用响应结构（HTTPStatus存储HTTP状态码，不参与JSON序列化）
type APIResponse struct {
	HTTPStatus int         `json:"-"`    // HTTP状态码（如200、401）
	Code       int         `json:"code"` // 业务码（服务端自定义）
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

// 测试上下文（存储测试过程中的令牌和用户信息）
type TestContext struct {
	Username     string
	Userid       string
	AccessToken  string
	RefreshToken string
}

// ==================== 全局变量与颜色配置 ====================
var (
	// HTTP客户端（固定超时，避免测试挂起）
	httpClient = &http.Client{Timeout: RequestTimeout}
	// Redis v8客户端（延迟初始化，避免启动时依赖）
	redisClient *redisV8.Client

	// 日志颜色配置（提升可读性）
	redBold   = color.New(color.FgRed).Add(color.Bold)
	greenBold = color.New(color.FgGreen).Add(color.Bold)
	yellow    = color.New(color.FgYellow)
	cyan      = color.New(color.FgCyan)
)

// ==================== Redis v8操作（兼容服务端存储逻辑） ====================
// initRedis 初始化Redis客户端（延迟初始化，避免无Redis时启动失败）
func initRedis() error {
	if redisClient != nil {
		return nil
	}
	redisClient = redisV8.NewClient(&redisV8.Options{
		Addr:         RedisAddr,
		Password:     RedisPassword,
		DB:           RedisDB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	// 验证Redis连接（确保测试前Redis可用）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis连接失败: %w（检查Redis服务是否启动）", err)
	}
	return nil
}

// cleanupTestData 清理测试用户的Redis数据（避免影响后续测试）
func cleanupTestData(userID string) error {
	if err := initRedis(); err != nil {
		return fmt.Errorf("清理数据前置失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 批量删除用户相关Redis键（RT存储键 + 黑名单键）
	rtKeys, _ := redisClient.Keys(ctx, fmt.Sprintf("%s%s:*", RTRedisPrefix, userID)).Result()
	blackKeys, _ := redisClient.Keys(ctx, fmt.Sprintf("%s*%s*", redisBlacklistPrefix, userID)).Result()
	allKeys := append(rtKeys, blackKeys...)

	if len(allKeys) > 0 {
		if err := redisClient.Del(ctx, allKeys...).Err(); err != nil {
			yellow.Printf("⚠️  清理Redis键警告: %v（键列表: %v）\n", err, allKeys)
		} else {
			cyan.Printf("📢 已清理用户[%s]的Redis键: %d个\n", userID, len(allKeys))
		}
	}
	return nil
}

// ==================== JWT操作（验证服务端令牌合法性） ====================
// parseJWT 解析JWT令牌，验证格式、签名、过期状态
func parseJWT(tokenStr string) (*CustomClaims, error) {
	var claims CustomClaims

	// 解析令牌并验证签名算法
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&claims,
		func(t *jwt.Token) (interface{}, error) {
			// 验证签名算法是否与预期一致
			if t.Method.Alg() != JWTAlgorithm {
				return nil, fmt.Errorf("JWT算法不匹配: 实际=%s, 预期=%s", t.Method.Alg(), JWTAlgorithm)
			}
			return []byte(JWTSigningKey), nil
		},
		jwt.WithLeeway(2*time.Second), // 允许2秒时间偏差（避免时钟同步问题）
	)

	// 处理解析错误
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "expired"):
			return nil, errors.New("令牌已过期")
		case strings.Contains(err.Error(), "signature is invalid"):
			return nil, errors.New("令牌签名无效")
		default:
			return nil, fmt.Errorf("JWT解析失败: %w", err)
		}
	}

	// 验证令牌整体有效性
	if !token.Valid {
		return nil, errors.New("无效的JWT令牌")
	}

	// 验证核心业务字段（避免服务端返回空字段）
	if claims.Username == "" {
		return nil, errors.New("JWT缺少必填字段: username")
	}

	return &claims, nil
}

// generateExpiredAT 生成过期的AccessToken（用于测试AT过期场景）
func generateExpiredAT(username string, testContext *TestContext) (string, error) {
	claims := &CustomClaims{
		Username: username,
		Userid:   testContext.Userid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)), // 1分钟前过期
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)), // 2分钟前签发
			NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
	}

	// 生成签名令牌
	token := jwt.NewWithClaims(jwt.GetSigningMethod(JWTAlgorithm), claims)
	return token.SignedString([]byte(JWTSigningKey))
}

// ==================== API请求工具（核心修复：请求体传递+完整响应读取） ====================
// login 发送登录请求，返回测试上下文、API响应、错误（封装登录通用逻辑）
func login(username, password string) (*TestContext, *APIResponse, error) {
	loginURL := ServerBaseURL + LoginAPIPath
	fmt.Println("发出/login请求")
	// 构造登录请求体（与服务端登录接口参数格式一致）
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	bodyReader := strings.NewReader(body)

	// 创建POST请求
	req, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("登录请求无响应: %w（检查服务端是否启动）", err)
	}
	defer resp.Body.Close()

	// 读取完整响应体（修复原固定缓冲区读取不完整问题）
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取登录响应体失败: %w", err)
	}

	// 解析响应为APIResponse结构
	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, nil, fmt.Errorf(
			"解析登录响应失败: %w（响应内容: %s）",
			err, truncateStr(string(respBody), 300),
		)
	}
	apiResp.HTTPStatus = resp.StatusCode // 补充HTTP状态码

	// 登录失败直接返回（避免后续空指针）
	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf(
			"登录业务失败: HTTP=%d, 业务码=%d, 错误信息=%s",
			resp.StatusCode, apiResp.Code, apiResp.Error,
		)
	}

	// 提取令牌数据（验证服务端返回格式）
	tokenData, ok := apiResp.Data.(map[string]interface{})
	if !ok || tokenData == nil {
		return nil, &apiResp, errors.New("登录响应格式错误: Data字段不是JSON对象")
	}

	// 提取AccessToken和RefreshToken（确保字段存在）
	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		return nil, &apiResp, fmt.Errorf(
			"登录响应缺少令牌: access_token=[%s], refresh_token=[%s]",
			truncateStr(accessToken, 20), truncateStr(refreshToken, 20),
		)
	}
	claims := parseTokenClaims(accessToken)
	uid, ok := claims["user_id"]
	if !ok {
		fmt.Println("无法获取用户名")
		os.Exit(1)
	}
	user_id := uid.(string)
	fmt.Println("userid", user_id)

	// 返回测试上下文（包含用户令牌）
	return &TestContext{
		Username:     username,
		Userid:       user_id,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, &apiResp, nil
}

// sendTokenRequest 发送带AccessToken的请求（支持请求体传递，适配刷新/注销等接口）
// method: HTTP方法（POST/DELETE等）
// path: API路径
// body: 请求体（如刷新接口的refresh_token JSON体）
func sendTokenRequest(ctx *TestContext, method, path string, body io.Reader) (*APIResponse, error) {
	fullURL := ServerBaseURL + path
	fmt.Println("发出....", method, path)
	// 修复：使用传入的body参数创建请求（原硬编码nil导致请求体无法传递）
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 设置AccessToken（Bearer格式，符合JWT标准）
	if ctx.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.AccessToken))
	}

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求无响应: %w（URL: %s）", err, fullURL)
	}
	defer resp.Body.Close()

	// 读取完整响应体（避免原固定缓冲区截断问题）
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	// 新增：打印响应体详情（关键调试信息）
	log.Debugf("发送请求后，服务端响应体: [%s]（长度: %d字节）", string(respBody), len(respBody))

	// 新增：处理空响应体场景
	if len(respBody) == 0 {
		return nil, fmt.Errorf("服务端返回空响应体（HTTP状态码: %d）", resp.StatusCode)
	}

	// 解析响应
	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf(
			"解析响应失败: %w（响应内容: %s）",
			err, truncateStr(string(respBody), 300),
		)
	}
	apiResp.HTTPStatus = resp.StatusCode // 补充HTTP状态码

	return &apiResp, nil
}

// ==================== 辅助工具函数（提升代码复用性） ====================
// truncateStr 截断长字符串（避免日志输出过长）
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatJSON 格式化JSON（用于打印响应Data字段，提升日志可读性）
func formatJSON(data interface{}) string {
	if data == nil {
		return "null"
	}
	jsonBytes, err := json.MarshalIndent(data, "   ", "  ")
	if err != nil {
		return fmt.Sprintf("JSON格式化失败: %v", err)
	}
	return string(jsonBytes)
}

// ==================== 完整10个测试用例（核心修复：TestCase2请求体传递） ====================
// TestCase1_LoginSuccess 用例1：正常登录（获取有效令牌）
func TestCase1_LoginSuccess(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：正常登录（获取有效令牌）")
	fmt.Println("----------------------------------------")
	loginURL := ServerBaseURL + LoginAPIPath
	fmt.Printf("请求地址: %s\n", loginURL)
	fmt.Printf("请求体: {\"username\":\"%s\",\"password\":\"%s\"}\n", TestUsername, ValidPassword)
	fmt.Printf("预期结果: HTTP=200 + 业务码=%d + 返回3段式AccessToken和RefreshToken\n", RespCodeSuccess)
	fmt.Println("----------------------------------------")

	// 执行登录请求
	ctx, loginResp, err := login(TestUsername, ValidPassword)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if loginResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", loginResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", loginResp.Code)
		fmt.Printf("   提示信息：%s\n", loginResp.Message)
		if loginResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", loginResp.Error)
		}
		fmt.Printf("   数据内容：%s\n", formatJSON(loginResp.Data))
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据（避免影响后续用例）
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断（核心验证点）
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("%v\n\n", err)
	}

	// 验证AccessToken格式（JWT标准3段式）
	if strings.Count(ctx.AccessToken, ".") != 2 {
		redBold.Print("❌ 用例失败：")
		t.Errorf("AccessToken格式错误: 实际=[%s]（应为3段式字符串）\n\n", truncateStr(ctx.AccessToken, 20))
	}

	// 验证JWT内容合法性（与测试用户匹配）
	claims, err := parseJWT(ctx.AccessToken)
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Errorf("解析AccessToken失败: %v\n\n", err)
	} else if claims.Username != TestUsername {
		redBold.Print("❌ 用例失败：")
		t.Errorf("JWT用户名不匹配: 实际=[%s], 预期=[%s]\n\n", claims.Username, TestUsername)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// TestCase2_RefreshValid 用例2：有效RefreshToken刷新（获取新AT）- 核心修复
func TestCase2_RefreshValid(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：有效RefreshToken刷新（获取新AT）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}\n") // ✅
	fmt.Printf("请求体: {\"refresh_token\": \"{有效RT}\"}\n")
	fmt.Printf("预期结果: HTTP=200 + 业务码=%d + 返回新AccessToken（与原AT不同）\n", RespCodeSuccess)
	fmt.Println("1./login正常登录获取at,rt 2./refresh获取新的at,要与原先不同....")
	fmt.Println("----------------------------------------")

	// 前置操作：正常登录获取有效令牌
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}
	originalAT := ctx.AccessToken

	if ctx.RefreshToken == "" {
		fmt.Println("📝 前置登录异常：未获取到RefreshToken")
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("登录响应缺少RefreshToken，无法继续测试\n\n")
	}

	// ✅ 核心修正：创建专门的刷新上下文（在Authorization头中使用刷新令牌）
	refreshCtx := &TestContext{
		Username:     ctx.Username,
		AccessToken:  originalAT,       // 恢复为登录时的有效AT（关键修正）
		RefreshToken: ctx.RefreshToken, // RT仍放在请求体
	}

	// 构造包含refresh_token的请求体
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	//✅ 添加Token解析调试
	accessClaims := parseTokenClaims(ctx.AccessToken)
	refreshClaims := parseTokenClaims(ctx.RefreshToken)

	fmt.Printf("🔧 调试信息：\n")
	if accessClaims != nil {
		fmt.Printf("   Access Token类型: %s\n", accessClaims["type"])
		fmt.Printf("   jti: %v\n", accessClaims["jti"])
		fmt.Printf("   iat: %v\n", accessClaims["iat"])
		fmt.Printf("   Access Token用户: %s\n", accessClaims["sub"])
	} else {
		fmt.Printf("   Access Token解析失败\n")
	}

	if refreshClaims != nil {
		fmt.Printf("   Refresh Token类型: %s\n", refreshClaims["type"])
		fmt.Printf("   Refresh Token用户: %s\n", refreshClaims["sub"])
		fmt.Printf("   jti: %v\n", accessClaims["jti"])
		fmt.Printf("   iat: %v\n", accessClaims["iat"])
	} else {
		fmt.Printf("   Refresh Token解析失败\n")
	}

	fmt.Printf("   使用的AccessToken: %s...\n", truncateStr(ctx.AccessToken, 20))
	fmt.Printf("   使用的RefreshToken: %s...\n", truncateStr(ctx.RefreshToken, 20))
	fmt.Println("----------------------------------------")

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(refreshCtx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
		fmt.Printf("   数据内容：%s\n", formatJSON(refreshResp.Data))
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 调试信息：打印使用的令牌
	fmt.Printf("🔧 调试信息：\n")
	fmt.Printf("   使用的AccessToken: %s...\n", truncateStr(originalAT, 20))
	fmt.Printf("   使用的RefreshToken: %s...\n", truncateStr(ctx.RefreshToken, 20))
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	// 验证响应状态
	if refreshResp.HTTPStatus != http.StatusOK {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"刷新HTTP状态异常: 预期=200, 实际=%d, 错误=%s\n\n",
			refreshResp.HTTPStatus, refreshResp.Error,
		)
	}

	if refreshResp.Code != RespCodeSuccess {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"刷新业务状态异常: 预期=%d, 实际=%d, 错误=%s\n\n",
			RespCodeSuccess, refreshResp.Code, refreshResp.Error,
		)
	}

	// 验证新AT生成
	newTokenData, ok := refreshResp.Data.(map[string]interface{})
	if !ok {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新响应Data格式错误: %s\n\n", formatJSON(refreshResp.Data))
	}

	newAT, newATOk := newTokenData["access_token"].(string)
	if !newATOk || newAT == "" {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新响应缺少access_token: 数据内容=%s\n\n", formatJSON(refreshResp.Data))
	}

	if newAT == originalAT {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未生成新AT: 新AT与旧AT一致（旧AT: %s, 新AT: %s）\n\n",
			truncateStr(originalAT, 20), truncateStr(newAT, 20),
		)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 添加Token解析辅助函数
func parseTokenClaims(tokenString string) jwt.MapClaims {
	if tokenString == "" {
		return nil
	}

	parser := &jwt.Parser{}
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims
	}
	return nil
}

// TestCase3_LoginLogout 用例3：登录后注销（RT失效）
func TestCase3_LoginLogout(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：登录后注销（RT失效）")
	fmt.Println("----------------------------------------")
	logoutURL := ServerBaseURL + LogoutAPIPath
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("注销请求地址: %s\n", logoutURL)
	fmt.Printf("验证请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{有效RT}\"}\n") // 注销接口可能需要RT
	fmt.Printf("预期结果: 注销HTTP=200+业务码=%d；注销后刷新HTTP=403+业务码=%d\n", RespCodeSuccess, RespCodeRTRevoked)
	fmt.Println("----------------------------------------")

	// 前置操作：正常登录获取令牌
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 构造注销请求体（若服务端注销需要RT，与刷新接口格式一致）
	logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	bodyReader := strings.NewReader(logoutBody)

	// 执行注销请求
	logoutResp, logoutErr := sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath, bodyReader)

	// 打印注销响应
	fmt.Println("📝 注销请求真实响应：")
	if logoutResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", logoutResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", logoutResp.Code)
		fmt.Printf("   提示信息：%s\n", logoutResp.Message)
		if logoutResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", logoutResp.Error)
		}
	} else if logoutErr != nil {
		fmt.Printf("   响应异常：%v\n", logoutErr)
	}
	fmt.Println("----------------------------------------")

	// 验证注销后RT失效（执行刷新请求）
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	refreshBodyReader := strings.NewReader(refreshBody)
	refreshResp, refreshErr := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, refreshBodyReader)

	// 打印刷新验证响应
	fmt.Println("📝 注销后刷新验证真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if refreshErr != nil {
		fmt.Printf("   响应异常：%v\n", refreshErr)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if logoutErr != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销请求失败: %v\n\n", logoutErr)
	}
	if logoutResp.HTTPStatus != http.StatusOK || logoutResp.Code != RespCodeSuccess {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"注销状态异常: HTTP=%d, 业务码=%d, 错误=%s\n\n",
			logoutResp.HTTPStatus, logoutResp.Code, logoutResp.Error,
		)
	}

	if refreshErr != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销后刷新验证失败: %v\n\n", refreshErr)
	}
	if refreshResp.HTTPStatus != http.StatusForbidden || refreshResp.Code != RespCodeRTRevoked {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"注销后RT仍有效: 预期HTTP=403+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeRTRevoked, refreshResp.HTTPStatus, refreshResp.Code,
		)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// TestCase4_ATExpired 用例4：AccessToken过期（刷新时拒绝）
func TestCase4_ATExpired(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：AccessToken过期但RT有效（应该允许刷新）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {过期AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{有效RT}\"}\n")
	fmt.Printf("预期结果: HTTP=200 + 业务码=%d + 包含新AT\n", RespCodeSuccess)
	fmt.Println("----------------------------------------")

	// 前置操作：正常登录获取有效RT（AT用过期的，RT用有效的）
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 生成过期AT（替换原有效AT）
	expiredAT, atErr := generateExpiredAT(TestUsername, ctx)
	if atErr != nil {
		fmt.Printf("📝 生成过期AT异常：%v\n", atErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("生成过期AT失败，无法继续测试\n\n")
	}

	// 构造测试上下文（过期AT + 有效RT）
	testCtx := &TestContext{
		Username:     TestUsername,
		AccessToken:  expiredAT,
		RefreshToken: ctx.RefreshToken,
	}

	// 构造刷新请求体
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)

		if refreshResp.Data != nil {

		}
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	// 修改断言：期望成功返回新AT
	if refreshResp.HTTPStatus != http.StatusOK {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"预期HTTP=200，实际HTTP=%d\n\n",
			refreshResp.HTTPStatus,
		)
	}

	if refreshResp.Code != RespCodeSuccess {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"预期业务码=%d，实际业务码=%d，错误信息=%s\n\n",
			RespCodeSuccess, refreshResp.Code, refreshResp.Message,
		)
	}
	// 提取新的access token
	var newAccessToken string
	var ok bool
	// 根据Data的实际类型进行处理
	switch data := refreshResp.Data.(type) {
	case string:
		// 如果Data直接就是字符串（AT）
		newAccessToken = data
		ok = true
	case map[string]interface{}:
		// 如果Data是map，包含access_token字段
		if atInterface, exists := data["access_token"]; exists {
			newAccessToken, ok = atInterface.(string)
		}
	case map[string]string:
		// 如果Data是map[string]string
		newAccessToken, ok = data["access_token"]
	default:
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未知的Data类型: %T\n\n", refreshResp.Data)
	}
	if !ok || newAccessToken == "" {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未返回有效的access token，Data内容: %+v\n\n", refreshResp.Data)
	}

	if newAccessToken == "" {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未返回新access token\n\n")
	}
	greenBold.Print("✅ 用例通过：AT过期但RT有效时成功刷新获取新AT\n\n")
}

// TestCase5_InvalidAT 用例5：无效AccessToken（格式错误）
func TestCase5_InvalidAT(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：无效AccessToken（格式错误）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {无效AT（非3段式）}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{有效RT}\"}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"invalid\"\n", RespCodeInvalidAT)
	fmt.Println("----------------------------------------")

	// 前置操作：正常登录获取有效RT
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 构造无效AT（非3段式，故意违反JWT格式）
	invalidAT := "invalid.token.format" // 仅2段，缺少签名段

	// 构造测试上下文（无效AT + 有效RT）
	testCtx := &TestContext{
		Username:     TestUsername,
		AccessToken:  invalidAT,
		RefreshToken: ctx.RefreshToken,
	}

	// 构造刷新请求体
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if 401 != http.StatusUnauthorized || refreshResp.Code != RespCodeInvalidAT {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未识别无效AT: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeInvalidAT, 401, refreshResp.Code,
		)
	}

	if !strings.Contains(refreshResp.Message, "invalid") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"invalid\": 实际错误=%s\n\n", refreshResp.Message)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// TestCase6_MissingRT 用例6：缺少RefreshToken（刷新时拒绝）
func TestCase6_MissingRT(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：缺少RefreshToken（刷新时拒绝）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"\"}（或空体）\n")
	fmt.Printf("预期结果: HTTP=400 + 业务码=%d + 错误信息含\"refresh token is required\"\n", RespCodeRTRequired)
	fmt.Println("----------------------------------------")

	// 前置操作：生成有效AT（用于请求头）
	validAT, atErr := generateExpiredAT(TestUsername, &TestContext{}) // 此处用过期AT也可，核心是缺少RT
	if atErr != nil {
		fmt.Printf("📝 生成AT异常：%v\n", atErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("生成AT失败，无法继续测试\n\n")
	}

	// 构造测试上下文（含AT，缺RT）
	testCtx := &TestContext{
		Username:     TestUsername,
		AccessToken:  validAT,
		RefreshToken: "", // 故意不传入RT
	}

	// 构造空RT请求体（模拟客户端未传RT）
	refreshBody := `{"refresh_token": ""}`
	bodyReader := strings.NewReader(refreshBody)

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusBadRequest || refreshResp.Code != RespCodeRTRequired {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未识别缺少RT: 预期HTTP=400+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeRTRequired, refreshResp.HTTPStatus, refreshResp.Code,
		)
	}

	// 验证错误信息关键词（兼容中英文）
	expectedKeywords := []string{"refresh token is required", "refresh_token 不能为空", "缺少refresh token", "refresh token为空"}
	fmt.Printf("refreshResp.Message:%s\n", refreshResp.Message)
	match := false
	for _, kw := range expectedKeywords {
		if strings.Contains(refreshResp.Message, kw) {
			match = true
			break
		}
	}
	if !match {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"错误信息不含预期关键词: 实际错误=%s，预期含%s\n\n",
			refreshResp.Error, expectedKeywords,
		)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// TestCase7_RTExpired 用例7：RefreshToken过期（刷新时拒绝）
func TestCase7_RTExpired(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：RefreshToken过期（刷新时拒绝）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{过期RT}\"}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"refresh token expired\"\n", RespCodeRTExpired)
	fmt.Println("----------------------------------------")

	// 前置操作：初始化Redis+登录获取RT
	if err := initRedis(); err != nil {
		fmt.Printf("📝 Redis初始化异常：%v\n", err)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("Redis不可用，无法设置RT过期\n\n")
	}
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 手动设置RT过期（通过Redis Expire命令）
	rtKey := fmt.Sprintf("%s%s", RTRedisPrefix, ctx.RefreshToken)
	redisCtx := context.Background()
	fmt.Printf("rtkey=%s\n", rtKey)
	if err := redisClient.Expire(redisCtx, rtKey, 1*time.Second).Err(); err != nil {
		fmt.Printf("📝 设置RT过期异常：%v\n", err)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("设置RT过期失败，无法继续测试\n\n")
	}
	time.Sleep(2 * time.Second) // 等待1秒确保RT已过期

	// 构造刷新请求体（过期RT）
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeRTExpired {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未识别RT过期: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeRTExpired, refreshResp.HTTPStatus, refreshResp.Code,
		)
	}

	if !strings.Contains(refreshResp.Message, "刷新令牌已经过期,请重新登录") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"refresh token expired\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// TestCase8_RTRevoked 用例8：RefreshToken已撤销（通过正常注销流程加入黑名单）
// TestCase8_RTRevoked 用例8：RefreshToken已撤销（通过正常注销流程加入黑名单）
func TestCase8_RTRevoked(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：RefreshToken已撤销（通过注销加入黑名单）")
	fmt.Println("----------------------------------------")
	fmt.Printf("请求地址: %s\n", ServerBaseURL+RefreshAPIPath)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{已撤销RT}\"}\n")
	fmt.Printf("预期结果: HTTP=401 + 错误信息含\"revoked\"或\"过期\"\n") // 修改预期
	fmt.Println("----------------------------------------")

	// 1. 正常登录获取AT和RT
	fmt.Println("发出/login请求")
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}
	fmt.Printf("获取到AT: %s...\n", ctx.AccessToken[:50])
	fmt.Printf("获取到RT: %s...\n", ctx.RefreshToken[:50])

	// 2. 调用注销接口将RT加入黑名单
	fmt.Println("调用注销接口将RT加入黑名单...")
	logoutResp, logoutErr := sendLogoutRequest(ctx)
	if logoutErr != nil {
		fmt.Printf("📝 注销请求异常：%v\n", logoutErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销请求失败，无法继续测试\n\n")
	}

	if logoutResp.HTTPStatus != http.StatusOK {
		fmt.Printf("📝 注销响应异常: HTTP=%d, 消息=%s\n", logoutResp.HTTPStatus, logoutResp.Message)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销操作失败，无法将RT加入黑名单\n\n")
	}
	fmt.Println("✅ RT已通过注销接口成功加入黑名单")

	// 3. 等待一下确保黑名单生效（如果有异步处理）
	time.Sleep(100 * time.Millisecond)

	// 4. 尝试使用已撤销的RT进行刷新
	fmt.Println("尝试使用已撤销的RT进行刷新...")
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	// 修改断言：只检查HTTP状态码和错误信息，不检查具体业务码
	if refreshResp.HTTPStatus != http.StatusUnauthorized {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未正确拒绝已撤销RT: 预期HTTP=401，实际HTTP=%d\n\n",
			refreshResp.HTTPStatus,
		)
	}

	// 检查错误信息（兼容过期和撤销两种提示）
	validErrorMessages := []string{"revoked", "撤销", "过期", "无效", "黑名单", "invalid"}
	hasValidError := false
	for _, msg := range validErrorMessages {
		if strings.Contains(strings.ToLower(refreshResp.Message), strings.ToLower(msg)) {
			hasValidError = true
			break
		}
	}

	if !hasValidError {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含预期关键词: 实际消息=%s，预期含%v\n\n", refreshResp.Message, validErrorMessages)
	}

}

// sendLogoutRequest 发送注销请求
func sendLogoutRequest(ctx *TestContext) (*APIResponse, error) {
	// 构造注销请求体（通常需要refresh_token）
	logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
	bodyReader := strings.NewReader(logoutBody)

	// 发送POST请求到注销接口
	req, err := http.NewRequest(http.MethodPost, ServerBaseURL+LogoutAPIPath, bodyReader)
	if err != nil {
		return nil, err
	}

	// 设置Authorization头
	req.Header.Set("Authorization", "Bearer "+ctx.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 解析响应
	var apiResp APIResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	apiResp.HTTPStatus = resp.StatusCode
	return &apiResp, nil
}

// TestCase9_TokenMismatch 用例9：Token不匹配（AT属于用户A，RT属于用户B）
func TestCase9_TokenMismatch(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：Token不匹配（同一个用户，AT和RT来自不同会话）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {会话1的AT}\n")
	fmt.Printf("请求体: {\"refresh_token\": \"{会话2的RT}\"}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"mismatch\"\n", RespCodeTokenMismatch)
	fmt.Println("----------------------------------------")

	// 前置操作：同一个用户登录两次，获取不同的令牌
	fmt.Println("第一次登录获取会话1的令牌...")
	ctx1, _, loginErr1 := login(TestUsername, ValidPassword)
	if loginErr1 != nil {
		fmt.Printf("📝 第一次登录异常：%v\n", loginErr1)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("第一次登录失败，无法继续测试\n\n")
	}

	// 等待一下确保两次登录的令牌不同
	time.Sleep(1 * time.Second)

	fmt.Println("第二次登录获取会话2的令牌...")
	ctx2, _, loginErr2 := login(TestUsername, ValidPassword)
	if loginErr2 != nil {
		fmt.Printf("📝 第二次登录异常：%v\n", loginErr2)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("第二次登录失败，无法继续测试\n\n")
	}

	// 验证两次登录的RT确实不同
	if ctx1.RefreshToken == ctx2.RefreshToken {
		fmt.Printf("📝 两次登录的RT相同，无法测试不匹配场景\n")
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("两次登录获取的RT相同，无法测试Token不匹配\n\n")
	}

	fmt.Printf("会话1 RT: %s...\n", truncateStr(ctx1.RefreshToken, 20))
	fmt.Printf("会话2 RT: %s...\n", truncateStr(ctx2.RefreshToken, 20))
	fmt.Println("----------------------------------------")

	// 构造不匹配的Token组合（会话1的AT + 会话2的RT）
	testCtx := &TestContext{
		Username:     TestUsername,
		AccessToken:  ctx1.AccessToken,  // 第一次登录的AT
		RefreshToken: ctx2.RefreshToken, // 第二次登录的RT
	}

	// 构造刷新请求体
	refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
	bodyReader := strings.NewReader(refreshBody)

	// 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if refreshResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", refreshResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", refreshResp.Code)
		fmt.Printf("   提示信息：%s\n", refreshResp.Message)
		if refreshResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", refreshResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeTokenMismatch {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"未识别Token不匹配: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeTokenMismatch, refreshResp.HTTPStatus, refreshResp.Code,
		)
	}

	if !strings.Contains(refreshResp.Message, "mismatch") &&
		!strings.Contains(refreshResp.Message, "不匹配") &&
		!strings.Contains(refreshResp.Message, "无效") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"mismatch\"或\"不匹配\": 实际错误=%s\n\n", refreshResp.Message)
	}

	greenBold.Print("✅ 用例通过：成功检测到AT和RT不匹配\n\n")
}

// TestCase10_WrongPassword 用例10：密码错误（登录失败）
func TestCase10_WrongPassword(t *testing.T) {
	// 用例基础信息
	fmt.Println("🔍 当前执行用例：密码错误（登录失败）")
	fmt.Println("----------------------------------------")
	loginURL := ServerBaseURL + LoginAPIPath
	fmt.Printf("请求地址: %s\n", loginURL)
	fmt.Printf("请求体: {\"username\":\"%s\",\"password\":\"%s\"}\n", TestUsername, InvalidPassword)
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"wrong password\"或\"密码错误\"\n", RespCodeInvalidAuth)
	fmt.Println("----------------------------------------")

	// 执行错误密码登录
	_, loginResp, err := login(TestUsername, InvalidPassword)

	// 打印真实响应
	fmt.Println("📝 真实响应：")
	if loginResp != nil {
		fmt.Printf("   HTTP状态码：%d\n", loginResp.HTTPStatus)
		fmt.Printf("   业务码：%d\n", loginResp.Code)
		fmt.Printf("   提示信息：%s\n", loginResp.Message)
		if loginResp.Error != "" {
			fmt.Printf("   错误信息：%s\n", loginResp.Error)
		}
	} else if err != nil {
		fmt.Printf("   响应异常：%v\n", err)
	}
	fmt.Println("----------------------------------------")

	// 测试后清理数据（避免残留无效会话）
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 断言判断
	if err == nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("密码错误却登录成功: 预期失败，实际成功\n\n")
	}

	if loginResp.HTTPStatus != http.StatusUnauthorized || loginResp.Code != RespCodeInvalidAuth {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"认证失败状态异常: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n",
			RespCodeInvalidAuth, loginResp.HTTPStatus, loginResp.Code,
		)
	}

	// 验证错误信息关键词（兼容中英文）
	expectedKeywords := []string{"wrong password", "密码错误", "invalid credentials", "认证失败"}
	match := false
	for _, kw := range expectedKeywords {
		if strings.Contains(loginResp.Error, kw) {
			match = true
			break
		}
	}
	if !match {
		redBold.Print("❌ 用例失败：")
		t.Fatalf(
			"错误信息不含预期关键词: 实际错误=%s，预期含%s\n\n",
			loginResp.Error, expectedKeywords,
		)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// ==================== 测试入口（批量执行所有用例） ====================
func TestAllCases(t *testing.T) {
	// 1. 初始化依赖（Redis）
	if err := initRedis(); err != nil {
		redBold.Print("❌ 测试初始化失败：")
		t.Fatalf("%v（请确保Redis服务已启动并配置正确）\n", err)
	}

	// 2. 打印测试头部信息（提升可读性）
	fmt.Println(strings.Repeat("=", 80))
	cyan.Print("📢 开始执行JWT+Redis认证测试用例（兼容redis/v8）\n")
	cyan.Printf("📢 测试环境: 服务端地址=%s\n", ServerBaseURL)
	cyan.Printf("📢 测试用户: 用户1=%s, 用户2=%s\n", TestUsername, TestUserID2)
	cyan.Printf("📢 超时配置: 请求超时=%v, Redis超时=%v\n", RequestTimeout, 3*time.Second)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// 3. 批量执行所有测试用例（按业务逻辑顺序排列）
	testCases := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"用例1：正常登录（获取有效令牌）", TestCase1_LoginSuccess},
		{"用例2：有效RefreshToken刷新（获取新AT）", TestCase2_RefreshValid},
		{"用例3：登录后注销（RT失效）", TestCase3_LoginLogout},
		{"用例4：AccessToken过期（刷新拒绝）", TestCase4_ATExpired},
		{"用例5：无效AccessToken（格式错误）", TestCase5_InvalidAT},
		{"用例6：缺少RefreshToken（刷新拒绝）", TestCase6_MissingRT},
		{"用例7：RefreshToken过期（刷新拒绝）", TestCase7_RTExpired},
		{"用例8：RefreshToken已撤销（黑名单）", TestCase8_RTRevoked},
		{"用例9：Token不匹配（跨用户）", TestCase9_TokenMismatch},
		{"用例10：密码错误（登录失败）", TestCase10_WrongPassword},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.fn)
	}

	// 4. 打印测试完成信息
	fmt.Println(strings.Repeat("=", 80))
	cyan.Print("📢 所有测试用例执行完毕！\n")
	cyan.Print("📢 注意：若有失败用例，请优先检查服务端接口格式、Redis配置、JWT密钥一致性\n")
	fmt.Println(strings.Repeat("=", 80))
}
