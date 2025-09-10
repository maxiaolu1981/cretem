package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	redisV8 "github.com/go-redis/redis/v8"
	jwt "github.com/golang-jwt/jwt/v5"
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
	InvalidPassword = "Admin@2022" // 修正：原无效密码与有效密码一致，此处修改为不同值

	// JWT配置
	JWTSigningKey   = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo" // 与框架JWT密钥一致
	JWTAlgorithm    = "HS256"
	TokenExpireTime = 60 * time.Second
	RTExpireTime    = 3600 * time.Second

	// Redis键前缀（与框架存储规则一致）
	RTRedisPrefix        = "gin-jwt:refresh:"
	redisBlacklistPrefix = "gin-jwt:blacklist:"

	// 业务码常量（根据实际接口返回调整）
	RespCodeSuccess       = 100001 // 成功
	RespCodeRTRequired    = 100001 // 缺少RefreshToken
	RespCodeRTRevoked     = 100002 // RefreshToken已撤销
	RespCodeATExpired     = 100003 // AccessToken已过期
	RespCodeInvalidAT     = 100004 // AccessToken无效
	RespCodeRTExpired     = 100005 // RefreshToken已过期
	RespCodeTokenMismatch = 100006 // Token不匹配
	RespCodeInvalidAuth   = 100007 // 认证失败（密码错误等）
)

// ==================== 数据结构定义（核心修正：区分HTTP状态码与业务码） ====================
// JWT自定义声明（适配gin-jwt）
type CustomClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// 令牌响应结构（匹配框架返回格式）
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// API通用响应结构（新增HTTPStatus存储HTTP状态码，Code保留为业务码）
type APIResponse struct {
	HTTPStatus int         `json:"-"`    // HTTP状态码（不参与JSON序列化）
	Code       int         `json:"code"` // 业务码
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

// 测试上下文（存储测试过程中的令牌和用户信息）
type TestContext struct {
	UserID       string
	AccessToken  string
	RefreshToken string
}

// ==================== 全局变量与颜色配置 ====================
var (
	// HTTP客户端（固定超时）
	httpClient = &http.Client{Timeout: RequestTimeout}
	// Redis v8客户端（延迟初始化）
	redisClient *redisV8.Client

	// 颜色配置
	redBold   = color.New(color.FgRed).Add(color.Bold)
	greenBold = color.New(color.FgGreen).Add(color.Bold)
	yellow    = color.New(color.FgYellow)
	cyan      = color.New(color.FgCyan)
)

// ==================== Redis v8操作（兼容框架） ====================
func initRedis() error {
	if redisClient != nil {
		return nil
	}
	redisClient = redisV8.NewClient(&redisV8.Options{
		Addr:        RedisAddr,
		Password:    RedisPassword,
		DB:          RedisDB,
		DialTimeout: 3 * time.Second,
	})

	// 验证Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return redisClient.Ping(ctx).Err()
}

// 清理测试数据（避免影响后续测试）
func cleanupTestData(userID string) error {
	if err := initRedis(); err != nil {
		return fmt.Errorf("清理数据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 批量删除用户相关的Redis键
	rtKeys, _ := redisClient.Keys(ctx, fmt.Sprintf("%s%s:*", RTRedisPrefix, userID)).Result()
	blackKeys, _ := redisClient.Keys(ctx, fmt.Sprintf("%s*%s*", redisBlacklistPrefix, userID)).Result()
	allKeys := append(rtKeys, blackKeys...)

	if len(allKeys) > 0 {
		if err := redisClient.Del(ctx, allKeys...).Err(); err != nil {
			yellow.Printf("⚠️  清理Redis键警告: %v\n", err)
		}
	}

	cyan.Printf("📢 清理用户[%s]的Redis数据\n", userID)
	return nil
}

// ==================== JWT操作 ====================
// 解析JWT令牌
func parseJWT(tokenStr string) (*CustomClaims, error) {
	var claims CustomClaims

	token, err := jwt.ParseWithClaims(
		tokenStr,
		&claims,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != JWTAlgorithm {
				return nil, fmt.Errorf("算法不匹配: 实际=%s, 预期=%s", t.Method.Alg(), JWTAlgorithm)
			}
			return []byte(JWTSigningKey), nil
		},
		jwt.WithLeeway(2*time.Second),
	)

	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			return nil, errors.New("令牌已过期")
		}
		if strings.Contains(err.Error(), "signature is invalid") {
			return nil, errors.New("签名无效")
		}
		return nil, fmt.Errorf("JWT解析失败: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("无效的JWT令牌")
	}

	if claims.Username == "" {
		return nil, errors.New("JWT缺少username字段")
	}

	return &claims, nil
}

// 生成过期的AccessToken
func generateExpiredAT(username string) (string, error) {
	claims := &CustomClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.GetSigningMethod(JWTAlgorithm), claims)
	return token.SignedString([]byte(JWTSigningKey))
}

// ==================== API请求工具（核心修正：返回完整APIResponse） ====================
// 登录请求（返回TestContext、APIResponse、error）
func login(userID, password string) (*TestContext, *APIResponse, error) {
	loginURL := ServerBaseURL + LoginAPIPath
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, userID, password)

	// 创建请求
	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("登录请求无响应: %w（检查服务器是否正常）", err)
	}
	defer resp.Body.Close()

	// 解析响应
	respBody := make([]byte, 1024*10)
	n, _ := resp.Body.Read(respBody)
	var apiResp APIResponse
	if err := json.Unmarshal(respBody[:n], &apiResp); err != nil {
		return nil, nil, fmt.Errorf("解析登录响应失败: %w（响应内容: %s）", err, truncateStr(string(respBody[:n]), 300))
	}

	// 补充HTTP状态码（区分业务码）
	apiResp.HTTPStatus = resp.StatusCode

	// 登录失败直接返回
	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf("登录失败: 状态码=%d, 业务码=%d, 错误信息=%s", resp.StatusCode, apiResp.Code, apiResp.Error)
	}

	// 提取令牌数据
	tokenData, ok := apiResp.Data.(map[string]interface{})
	if !ok || tokenData == nil {
		return nil, &apiResp, errors.New("登录响应格式错误: 数据字段不是JSON对象")
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		return nil, &apiResp, fmt.Errorf("登录响应缺少令牌: access_token=%s, refresh_token=%s", accessToken, refreshToken)
	}

	return &TestContext{
		UserID:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, &apiResp, nil
}

// 发送带令牌的请求（返回APIResponse、error）
func sendTokenRequest(ctx *TestContext, method, path string) (*APIResponse, error) {
	fullURL := ServerBaseURL + path
	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 设置请求头
	if ctx.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.AccessToken))
	}
	if ctx.RefreshToken != "" {
		req.Header.Set("Refresh-Token", ctx.RefreshToken)
	}

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求无响应: %w（URL: %s）", err, fullURL)
	}
	defer resp.Body.Close()

	// 解析响应
	respBody := make([]byte, 1024*10)
	n, _ := resp.Body.Read(respBody)
	var apiResp APIResponse
	if err := json.Unmarshal(respBody[:n], &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w（响应内容: %s）", err, truncateStr(string(respBody[:n]), 300))
	}

	// 补充HTTP状态码
	apiResp.HTTPStatus = resp.StatusCode
	return &apiResp, nil
}

// ==================== 辅助工具函数 ====================
// 截断长字符串
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// 格式化JSON（用于打印Data字段）
func formatJSON(data interface{}) string {
	if data == nil {
		return "null"
	}
	jsonBytes, err := json.MarshalIndent(data, "   ", "  ")
	if err != nil {
		return fmt.Sprintf("格式化为JSON失败: %v", err)
	}
	return string(jsonBytes)
}

// ==================== 完整10个测试用例（按指定格式输出） ====================
// 用例1：正常登录（获取有效令牌）
func TestCase1_LoginSuccess(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：正常登录（获取有效令牌）")
	fmt.Println("----------------------------------------")
	loginURL := ServerBaseURL + LoginAPIPath
	fmt.Printf("请求地址: %s\n", loginURL)
	fmt.Printf("预期结果: HTTP=200 + 业务码=%d + 返回有效AccessToken（3段式JWT）和RefreshToken\n", RespCodeSuccess)
	fmt.Println("----------------------------------------")

	// 2. 执行登录请求
	ctx, loginResp, err := login(TestUsername, ValidPassword)

	// 3. 打印真实响应
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

	// 4. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 5. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("%v\n\n", err)
	}

	// 验证AccessToken格式
	if strings.Count(ctx.AccessToken, ".") != 2 {
		redBold.Print("❌ 用例失败：")
		t.Errorf("AT格式错误: 实际=%s（应为3段式字符串）\n\n", truncateStr(ctx.AccessToken, 20))
	}

	// 验证JWT内容
	claims, err := parseJWT(ctx.AccessToken)
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Errorf("解析AT失败: %v\n\n", err)
	} else if claims.Username != TestUsername {
		redBold.Print("❌ 用例失败：")
		t.Errorf("UserName不匹配: 实际=%s, 预期=%s\n\n", claims.Username, TestUsername)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例2：有效RefreshToken刷新（获取新AT）
func TestCase2_RefreshValid(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：有效RefreshToken刷新（获取新AT）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}, Refresh-Token={有效RT}\n")
	fmt.Printf("预期结果: HTTP=200 + 业务码=%d + 返回新AccessToken（与原AT不同）\n", RespCodeSuccess)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录获取有效Token
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}
	originalAT := ctx.AccessToken

	// 3. 执行刷新请求
	refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath)

	// 4. 打印真实响应
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

	// 5. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 6. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	// 验证响应状态
	if refreshResp.HTTPStatus != http.StatusOK || refreshResp.Code != RespCodeSuccess {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新状态异常: HTTP=%d, 业务码=%d, 错误=%s\n\n", refreshResp.HTTPStatus, refreshResp.Code, refreshResp.Error)
	}

	// 验证新AT生成
	newTokenData, ok := refreshResp.Data.(map[string]interface{})
	if !ok {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新响应Data格式错误: %s\n\n", formatJSON(refreshResp.Data))
	}
	newAT := newTokenData["access_token"].(string)
	if newAT == "" || newAT == originalAT {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未生成新AT: 新AT为空或与旧AT一致（旧AT: %s）\n\n", truncateStr(originalAT, 20))
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例3：登录后注销（RT失效）
func TestCase3_LoginLogout(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：登录后注销（RT失效）")
	fmt.Println("----------------------------------------")
	logoutURL := ServerBaseURL + LogoutAPIPath
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("注销请求地址: %s\n", logoutURL)
	fmt.Printf("验证请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}, Refresh-Token={有效RT}\n")
	fmt.Printf("预期结果: 注销HTTP=200+业务码=%d；注销后刷新HTTP=401+业务码=%d\n", RespCodeSuccess, RespCodeRTRevoked)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 3. 执行注销请求
	logoutResp, logoutErr := sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath)

	// 4. 打印注销响应
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

	// 5. 验证注销后RT失效（执行刷新请求）
	refreshResp, refreshErr := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath)

	// 6. 打印刷新验证响应
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

	// 7. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 8. 断言判断
	if logoutErr != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销请求失败: %v\n\n", logoutErr)
	}
	if logoutResp.HTTPStatus != http.StatusOK || logoutResp.Code != RespCodeSuccess {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销状态异常: HTTP=%d, 业务码=%d, 错误=%s\n\n", logoutResp.HTTPStatus, logoutResp.Code, logoutResp.Error)
	}

	if refreshErr != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销后刷新验证失败: %v\n\n", refreshErr)
	}
	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeRTRevoked {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("注销后RT仍有效: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeRTRevoked, refreshResp.HTTPStatus, refreshResp.Code)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例4：AccessToken过期（刷新时拒绝）
func TestCase4_ATExpired(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：AccessToken过期（刷新时拒绝）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {过期AT}, Refresh-Token={有效RT}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"expired\"\n", RespCodeATExpired)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录获取有效RT
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 3. 生成过期AT
	expiredAT, atErr := generateExpiredAT(TestUsername)
	if atErr != nil {
		fmt.Printf("📝 生成过期AT异常：%v\n", atErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("生成过期AT失败，无法继续测试\n\n")
	}

	// 4. 构造测试上下文（过期AT+有效RT）
	testCtx := &TestContext{
		UserID:       TestUsername,
		AccessToken:  expiredAT,
		RefreshToken: ctx.RefreshToken,
	}

	// 5. 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath)

	// 6. 打印真实响应
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

	// 7. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 8. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeATExpired {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别AT过期: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeATExpired, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "expired") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"expired\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例5：无效AccessToken（格式错误）
func TestCase5_InvalidAT(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：无效AccessToken（格式错误）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {无效AT（非3段式）}, Refresh-Token={有效RT}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"invalid\"\n", RespCodeInvalidAT)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录获取有效RT
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 3. 构造无效AT（非3段式）
	invalidAT := "invalid.token" // 故意缺少1段

	// 4. 构造测试上下文
	testCtx := &TestContext{
		UserID:       TestUsername,
		AccessToken:  invalidAT,
		RefreshToken: ctx.RefreshToken,
	}

	// 5. 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath)

	// 6. 打印真实响应
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

	// 7. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 8. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeInvalidAT {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别无效AT: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeInvalidAT, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "invalid") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"invalid\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例6：缺少RefreshToken（刷新时拒绝）
func TestCase6_MissingRT(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：缺少RefreshToken（刷新时拒绝）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效过期AT}, Refresh-Token={空}\n")
	fmt.Printf("预期结果: HTTP=400 + 业务码=%d + 错误信息含\"refresh token is required\"\n", RespCodeRTRequired)
	fmt.Println("----------------------------------------")

	// 2. 生成有效过期AT
	validExpiredAT, atErr := generateExpiredAT(TestUsername)
	if atErr != nil {
		fmt.Printf("📝 生成过期AT异常：%v\n", atErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("生成过期AT失败，无法继续测试\n\n")
	}

	// 3. 构造测试上下文（含AT，缺RT）
	testCtx := &TestContext{
		UserID:       TestUsername,
		AccessToken:  validExpiredAT,
		RefreshToken: "", // 故意不传入RT
	}

	// 4. 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath)

	// 5. 打印真实响应
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

	// 6. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 7. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusBadRequest || refreshResp.Code != RespCodeRTRequired {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别缺少RT: 预期HTTP=400+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeRTRequired, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "refresh token is required") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"refresh token is required\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例7：RefreshToken过期（刷新时拒绝）
func TestCase7_RTExpired(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：RefreshToken过期（刷新时拒绝）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}, Refresh-Token={过期RT}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"refresh token expired\"\n", RespCodeRTExpired)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录获取RT
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 3. 手动设置RT过期（Redis操作）
	rtKey := fmt.Sprintf("%s%s:%s", RTRedisPrefix, TestUsername, ctx.RefreshToken)
	redisCtx := context.Background()
	if err := redisClient.Expire(redisCtx, rtKey, 1*time.Second).Err(); err != nil {
		fmt.Printf("📝 设置RT过期异常：%v\n", err)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("设置RT过期失败，无法继续测试\n\n")
	}
	time.Sleep(2 * time.Second) // 等待RT过期

	// 4. 执行刷新请求
	refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath)

	// 5. 打印真实响应
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

	// 6. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 7. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeRTExpired {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别RT过期: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeRTExpired, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "refresh token expired") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"refresh token expired\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例8：RefreshToken已撤销（加入黑名单）
func TestCase8_RTRevoked(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：RefreshToken已撤销（加入黑名单）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {有效AT}, Refresh-Token={已撤销RT}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"revoked\"\n", RespCodeRTRevoked)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：正常登录获取RT
	ctx, _, loginErr := login(TestUsername, ValidPassword)
	if loginErr != nil {
		fmt.Printf("📝 前置登录异常：%v\n", loginErr)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("前置登录失败，无法继续测试\n\n")
	}

	// 3. 手动将RT加入黑名单（模拟撤销）
	blackKey := fmt.Sprintf("%srt:%s", redisBlacklistPrefix, ctx.RefreshToken)
	redisCtx := context.Background()
	if err := redisClient.Set(redisCtx, blackKey, TestUsername, RTExpireTime).Err(); err != nil {
		fmt.Printf("📝 RT加入黑名单异常：%v\n", err)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("RT加入黑名单失败，无法继续测试\n\n")
	}

	// 4. 执行刷新请求
	refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath)

	// 5. 打印真实响应
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

	// 6. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 7. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeRTRevoked {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别已撤销RT: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeRTRevoked, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "revoked") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"revoked\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例9：Token不匹配（AT属于用户A，RT属于用户B）
func TestCase9_TokenMismatch(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：Token不匹配（AT属于用户A，RT属于用户B）")
	fmt.Println("----------------------------------------")
	refreshURL := ServerBaseURL + RefreshAPIPath
	fmt.Printf("请求地址: %s\n", refreshURL)
	fmt.Printf("请求头: Authorization=Bearer {用户1的AT}, Refresh-Token={用户2的RT}\n")
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"mismatch\"\n", RespCodeTokenMismatch)
	fmt.Println("----------------------------------------")

	// 2. 前置操作：两个用户分别登录
	ctx1, _, loginErr1 := login(TestUsername, ValidPassword)
	if loginErr1 != nil {
		fmt.Printf("📝 用户1登录异常：%v\n", loginErr1)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("用户1登录失败，无法继续测试\n\n")
	}

	ctx2, _, loginErr2 := login(TestUserID2, ValidPassword)
	if loginErr2 != nil {
		fmt.Printf("📝 用户2登录异常：%v\n", loginErr2)
		fmt.Println("----------------------------------------")
		redBold.Print("❌ 用例失败：")
		t.Fatalf("用户2登录失败，无法继续测试\n\n")
	}

	// 3. 构造不匹配的Token组合（用户1的AT + 用户2的RT）
	testCtx := &TestContext{
		UserID:       TestUsername,
		AccessToken:  ctx1.AccessToken,
		RefreshToken: ctx2.RefreshToken,
	}

	// 4. 执行刷新请求
	refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath)

	// 5. 打印真实响应
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

	// 6. 测试后清理数据
	defer func() {
		cleanupTestData(TestUsername)
		cleanupTestData(TestUserID2)
	}()

	// 7. 断言判断
	if err != nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("刷新请求失败: %v\n\n", err)
	}

	if refreshResp.HTTPStatus != http.StatusUnauthorized || refreshResp.Code != RespCodeTokenMismatch {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("未识别Token不匹配: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeTokenMismatch, refreshResp.HTTPStatus, refreshResp.Code)
	}

	if !strings.Contains(refreshResp.Error, "mismatch") {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含\"mismatch\": 实际错误=%s\n\n", refreshResp.Error)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// 用例10：密码错误（登录失败）
func TestCase10_WrongPassword(t *testing.T) {
	// 1. 打印用例基础信息
	fmt.Println("🔍 当前执行用例：密码错误（登录失败）")
	fmt.Println("----------------------------------------")
	loginURL := ServerBaseURL + LoginAPIPath
	fmt.Printf("请求地址: %s\n", loginURL)
	fmt.Printf("请求体: {\"user_id\":\"%s\",\"password\":\"%s\"}\n", TestUsername, InvalidPassword)
	fmt.Printf("预期结果: HTTP=401 + 业务码=%d + 错误信息含\"wrong password\"或\"密码错误\"\n", RespCodeInvalidAuth)
	fmt.Println("----------------------------------------")

	// 2. 执行错误密码登录
	_, loginResp, err := login(TestUsername, InvalidPassword)

	// 3. 打印真实响应
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

	// 4. 测试后清理数据
	defer func() {
		if cleanErr := cleanupTestData(TestUsername); cleanErr != nil {
			yellow.Printf("⚠️  清理用户[%s]数据失败：%v\n", TestUsername, cleanErr)
		}
	}()

	// 5. 断言判断
	if err == nil {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("密码错误却登录成功: 预期失败，实际成功\n\n")
	}

	if loginResp.HTTPStatus != http.StatusUnauthorized || loginResp.Code != RespCodeInvalidAuth {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("认证失败状态异常: 预期HTTP=401+业务码=%d，实际HTTP=%d+业务码=%d\n\n", RespCodeInvalidAuth, loginResp.HTTPStatus, loginResp.Code)
	}

	// 验证错误信息关键词
	expectedKeywords := []string{"wrong password", "密码错误", "invalid credentials"}
	match := false
	for _, kw := range expectedKeywords {
		if strings.Contains(loginResp.Error, kw) {
			match = true
			break
		}
	}
	if !match {
		redBold.Print("❌ 用例失败：")
		t.Fatalf("错误信息不含预期关键词: 实际错误=%s，预期含%s\n\n", loginResp.Error, expectedKeywords)
	}

	greenBold.Print("✅ 用例通过\n\n")
}

// ==================== 测试入口（批量执行所有用例） ====================
func TestAllCases(t *testing.T) {
	// 1. 初始化依赖（Redis）
	if err := initRedis(); err != nil {
		redBold.Print("❌ Redis初始化失败：")
		t.Fatalf("%v（检查Redis服务是否启动）\n", err)
	}

	// 2. 打印测试头部信息
	fmt.Println(strings.Repeat("=", 80))
	cyan.Print("📢 开始执行JWT+Redis测试用例（兼容redis/v8）\n")
	cyan.Printf("📢 测试环境: 服务器=%s, 测试用户1=%s, 测试用户2=%s\n", ServerBaseURL, TestUsername, TestUserID2)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// 3. 执行所有测试用例
	t.Run("用例1：正常登录", TestCase1_LoginSuccess)
	t.Run("用例2：有效RT刷新", TestCase2_RefreshValid)
	t.Run("用例3：登录后注销", TestCase3_LoginLogout)
	t.Run("用例4：AT过期", TestCase4_ATExpired)
	t.Run("用例5：无效AT", TestCase5_InvalidAT)
	t.Run("用例6：缺少RT", TestCase6_MissingRT)
	t.Run("用例7：RT过期", TestCase7_RTExpired)
	t.Run("用例8：已撤销RT", TestCase8_RTRevoked)
	t.Run("用例9：Token不匹配", TestCase9_TokenMismatch)
	t.Run("用例10：密码错误", TestCase10_WrongPassword)

	// 4. 打印测试完成信息
	fmt.Println(strings.Repeat("=", 80))
	cyan.Print("📢 所有测试用例执行完毕\n")
	fmt.Println(strings.Repeat("=", 80))
}
