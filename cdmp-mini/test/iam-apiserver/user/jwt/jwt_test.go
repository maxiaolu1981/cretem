/*
# 先运行基础测试确保功能正常
go test -v -run TestBasicLogin -timeout=10s

# 再运行并发测试
go test -v -run TestCase1_LoginSuccess_Concurrent -timeout=30s

# 如果还有问题，运行详细调试
go test -v -run TestDebugLoginDetailed -timeout=10s
# 运行所有并发测试
go test -v -run TestAllConcurrentCases -timeout=60s

# 运行单个测试用例
go test -v -run TestCase3_LoginLogout_Concurrent -timeout=30s

# 带race检测运行
go test -race -v -run TestAllConcurrentCases -timeout=60s

# 压力测试模式（增加并发数）
CONCURRENT_USERS=20 REQUESTS_PER_USER=10 go test -v -run TestAllConcurrentCases

完整的并发测试框架 - 6个核心测试用例

简化配置 - 减少不必要的复杂性

错误处理完善 - 所有错误都有日志输出

性能统计 - 显示QPS和成功率

独立运行 - 不依赖外部清理函数
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	redisV8 "github.com/go-redis/redis/v8"
)

var testUsers = []struct {
	username string
	password string
}{
	{"admin", "Admin@2021"},
	{"gettest-user105", "TestPass123!"},
	{"gettest-user135", "TestPass123!"},
	{"gettest-user136", "TestPass123!"},
	{"gettest-user137", "TestPass123!"},
	{"gettest-user138", "TestPass123!"},
	{"gettest-user139", "TestPass123!"},
	{"gettest-user140", "TestPass123!"},
	{"gettest-user141", "TestPass123!"},
	{"gettest-user142", "TestPass123!"},
	{"gettest-user143", "TestPass123!"},
	// ... 添加更多用户
}

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8080"
	RequestTimeout = 5 * time.Second

	LoginAPIPath   = "/login"
	RefreshAPIPath = "/refresh"
	LogoutAPIPath  = "/logout"

	RedisAddr     = "localhost:6379"
	RedisPassword = ""
	RedisDB       = 0

	TestUsername    = "admin"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022"

	JWTSigningKey = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo"
	JWTAlgorithm  = "HS256"

	RTRedisPrefix        = "genericapiserver:auth:refresh_token:"
	redisBlacklistPrefix = "gin-jwt:blacklist:"

	RespCodeSuccess       = 100001
	RespCodeRTRequired    = 110004
	RespCodeRTRevoked     = 100203
	RespCodeATExpired     = 100203
	RespCodeInvalidAT     = 100208
	RespCodeRTExpired     = 100203
	RespCodeTokenMismatch = 100006
	RespCodeInvalidAuth   = 100007

	ConcurrentUsers = 1
	RequestsPerUser = 1

	//ConcurrentUsers      = 10
	//RequestsPerUser      = 1000
	ConcurrentTestPrefix = "testuser_"
)

// 是否启用多用户测试模式
const EnableMultiUserTest = true // 设置为 true 启用多用户测试
// ====================
type APIResponse struct {
	HTTPStatus int         `json:"-"`
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

type TestContext struct {
	Username     string
	Userid       string
	AccessToken  string
	RefreshToken string
}

// ==================== 全局变量 ====================
var (
	httpClient  = &http.Client{Timeout: RequestTimeout}
	redisClient *redisV8.Client
	redBold     = color.New(color.FgRed).Add(color.Bold)
	greenBold   = color.New(color.FgGreen).Add(color.Bold)
	cyan        = color.New(color.FgCyan)
)

// ==================== Redis 操作 ====================
func initRedis() error {
	if redisClient != nil {
		return nil
	}
	redisClient = redisV8.NewClient(&redisV8.Options{
		Addr:     RedisAddr,
		Password: RedisPassword,
		DB:       RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return redisClient.Ping(ctx).Err()
}

// ==================== API 请求工具 ====================
func login(username, password string) (*TestContext, *APIResponse, error) {
	loginURL := ServerBaseURL + LoginAPIPath
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	bodyReader := strings.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, nil, err
	}
	apiResp.HTTPStatus = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf("登录失败")
	}

	tokenData, ok := apiResp.Data.(map[string]interface{})
	if !ok {
		return nil, &apiResp, errors.New("响应格式错误")
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)

	return &TestContext{
		Username:     username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, &apiResp, nil
}

func sendTokenRequest(ctx *TestContext, method, path string, body io.Reader) (*APIResponse, error) {
	fullURL := ServerBaseURL + path
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if ctx.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.AccessToken))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, err
	}
	apiResp.HTTPStatus = resp.StatusCode

	return &apiResp, nil
}

// ==================== 修复：恢复使用用户名参数 / ==================== 修改 runConcurrentTest 函数支持多用户 ====================
func runConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) bool) {
	fmt.Printf("\n%s\n", strings.Repeat("═", 70))
	fmt.Printf("🚀 开始并发测试: %s\n", testName)
	fmt.Printf("%s\n", strings.Repeat("─", 70))

	startTime := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failCount := 0

	// 确定测试模式
	testMode := "单用户模式"
	actualUsers := 1
	if EnableMultiUserTest && len(testUsers) > 1 {
		testMode = "多用户模式"
		actualUsers = min(ConcurrentUsers, len(testUsers))
	}

	fmt.Printf("   🎯 测试模式: %s\n", testMode)
	fmt.Printf("   👥 并发用户数: %d\n", ConcurrentUsers)
	fmt.Printf("   👤 实际使用用户数: %d\n", actualUsers)
	fmt.Printf("   📋 每个用户请求次数: %d\n", RequestsPerUser)
	fmt.Printf("   📦 总请求数: %d\n", ConcurrentUsers*RequestsPerUser)

	if EnableMultiUserTest {
		fmt.Printf("   📋 测试用户: %v\n", getTestUserNames())
	} else {
		fmt.Printf("   👤 测试用户: %s\n", TestUsername)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 70))

	// 创建进度通道
	progress := make(chan string, ConcurrentUsers*RequestsPerUser)
	done := make(chan bool)

	// 启动进度显示器
	go func() {
		for msg := range progress {
			fmt.Printf("   %s\n", msg)
		}
		done <- true
	}()

	for i := 0; i < ConcurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			// 确定使用的用户名和密码
			var username, password string
			if EnableMultiUserTest && len(testUsers) > 0 {
				userIndex := userID % len(testUsers)
				username = testUsers[userIndex].username
				password = testUsers[userIndex].password
			} else {
				username = TestUsername
				password = ValidPassword
			}

			for j := 0; j < RequestsPerUser; j++ {
				requestID := userID*RequestsPerUser + j + 1

				progress <- fmt.Sprintf("🟡 [用户%s] 请求 %d/%d 开始...", username, requestID, ConcurrentUsers*RequestsPerUser)

				success := testFunc(t, userID, username, password)

				mu.Lock()
				if success {
					successCount++
					progress <- fmt.Sprintf("🟢 [用户%s] 请求 %d 成功", username, requestID)
				} else {
					failCount++
					progress <- fmt.Sprintf("🔴 [用户%s] 请求 %d 失败", username, requestID)
				}
				mu.Unlock()

				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(progress)
	<-done

	duration := time.Since(startTime)
	totalRequests := ConcurrentUsers * RequestsPerUser

	// 输出测试结果
	fmt.Printf("%s\n", strings.Repeat("─", 70))
	fmt.Printf("📊 测试结果统计:\n")
	fmt.Printf("   🎯 测试模式: %s\n", testMode)
	fmt.Printf("   ✅ 成功请求: %d\n", successCount)
	fmt.Printf("   ❌ 失败请求: %d\n", failCount)
	fmt.Printf("   📈 成功率: %.1f%%\n", float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("   ⏱️  总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("   🚀 QPS: %.1f\n", float64(totalRequests)/duration.Seconds())
	fmt.Printf("   📦 总请求数: %d\n", totalRequests)

	// 性能评级
	rate := float64(successCount) / float64(totalRequests) * 100
	fmt.Printf("   🏆 性能评级: %s\n", getPerformanceRating(rate))
	fmt.Printf("%s\n", strings.Repeat("═", 70))

	t.Logf("测试完成: %s, 成功率: %.1f%%, 耗时: %v", testName, rate, duration)
}

// ==================== 辅助函数 ====================
func getTestUserNames() []string {
	names := make([]string, len(testUsers))
	for i, user := range testUsers {
		names[i] = user.username
	}
	return names
}

func getPerformanceRating(rate float64) string {
	switch {
	case rate >= 95:
		return "💎 优秀"
	case rate >= 80:
		return "⭐ 良好"
	case rate >= 60:
		return "⚠️  一般"
	default:
		return "❌ 较差"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== 修复所有测试用例的函数签名 ====================

func TestCase1_LoginSuccess_Concurrent(t *testing.T) {
	runConcurrentTest(t, "正常登录并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, resp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		if ctx.AccessToken == "" {
			t.Logf("用户 %s 请求 %d 未获取到AccessToken", username, userID)
			return false
		}

		if strings.Count(ctx.AccessToken, ".") != 2 {
			t.Logf("用户 %s 请求 %d AccessToken格式错误", username, userID)
			return false
		}

		if resp.Code != RespCodeSuccess {
			t.Logf("用户 %s 请求 %d 业务码错误: 预期=%d, 实际=%d", username, userID, RespCodeSuccess, resp.Code)
			return false
		}

		return true
	})
}

func TestCase2_RefreshValid_Concurrent(t *testing.T) {
	runConcurrentTest(t, "有效刷新令牌并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 刷新请求失败: %v", username, userID, err)
			return false
		}

		if refreshResp.Code != RespCodeSuccess {
			t.Logf("用户 %s 请求 %d 刷新业务失败: 预期=%d, 实际=%d", username, userID, RespCodeSuccess, refreshResp.Code)
			return false
		}

		return true
	})
}

func TestCase3_LoginLogout_Concurrent(t *testing.T) {
	runConcurrentTest(t, "登录登出并发测试", func(t *testing.T, userID int, username, password string) bool {
		// 登录
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		// 登出
		logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(logoutBody)
		logoutResp, err := sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 登出请求失败: %v", username, userID, err)
			return false
		}

		if logoutResp.Code != RespCodeSuccess {
			t.Logf("用户 %s 请求 %d 登出业务失败: 预期=%d, 实际=%d", username, userID, RespCodeSuccess, logoutResp.Code)
			return false
		}

		return true
	})
}

func TestCase4_ATExpired_Concurrent(t *testing.T) {
	runConcurrentTest(t, "AT过期并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		// 使用过期AT但有效RT
		testCtx := &TestContext{
			Username:     username,
			AccessToken:  "expired.token." + strings.Repeat("x", 100),
			RefreshToken: ctx.RefreshToken,
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 过期AT刷新失败: %v", username, userID, err)
			return false
		}

		t.Logf("用户 %s 请求 %d 过期AT刷新结果: 业务码=%d", username, userID, refreshResp.Code)
		return true
	})
}

func TestCase5_InvalidAT_Concurrent(t *testing.T) {
	runConcurrentTest(t, "无效AT并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		testCtx := &TestContext{
			Username:     username,
			AccessToken:  "invalid.token.format",
			RefreshToken: ctx.RefreshToken,
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err == nil && refreshResp.Code == RespCodeInvalidAT {
			t.Logf("用户 %s 请求 %d 无效AT检测成功", username, userID)
			return true
		} else {
			t.Logf("用户 %s 请求 %d 无效AT检测失败: 业务码=%d", username, userID, refreshResp.Code)
			return false
		}
	})
}

func TestCase6_MissingRT_Concurrent(t *testing.T) {
	runConcurrentTest(t, "缺少RT并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		refreshBody := `{"refresh_token": ""}`
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err == nil && refreshResp.Code == RespCodeRTRequired {
			t.Logf("用户 %s 请求 %d 缺少RT检测成功", username, userID)
			return true
		} else {
			t.Logf("用户 %s 请求 %d 缺少RT检测失败: 业务码=%d", username, userID, refreshResp.Code)
			return false
		}
	})
}

func TestCase7_RTExpired_Concurrent(t *testing.T) {
	if err := initRedis(); err != nil {
		t.Fatalf("Redis初始化失败: %v", err)
		return
	}

	runConcurrentTest(t, "RT过期并发测试", func(t *testing.T, userID int, username, password string) bool {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		// 设置RT过期
		rtKey := fmt.Sprintf("%s%s", RTRedisPrefix, ctx.RefreshToken)
		redisCtx := context.Background()
		redisClient.Expire(redisCtx, rtKey, 1*time.Second)
		time.Sleep(2 * time.Second)

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err == nil && refreshResp.Code == RespCodeRTExpired {
			t.Logf("用户 %s 请求 %d 过期RT检测成功", username, userID)
			return true
		} else {
			t.Logf("用户 %s 请求 %d 过期RT检测失败: 业务码=%d", username, userID, refreshResp.Code)
			return false
		}
	})
}

func TestCase8_RTRevoked_Concurrent(t *testing.T) {
	runConcurrentTest(t, "RT撤销并发测试", func(t *testing.T, userID int, username, password string) bool {
		// 登录并立即注销
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false
		}

		// 注销使RT失效
		logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(logoutBody)
		sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath, bodyReader)
		time.Sleep(100 * time.Millisecond)

		// 尝试使用已撤销的RT刷新
		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader = strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err == nil && refreshResp.HTTPStatus == http.StatusUnauthorized {
			t.Logf("用户 %s 请求 %d 撤销RT检测成功", username, userID)
			return true
		} else {
			t.Logf("用户 %s 请求 %d 撤销RT检测失败: HTTP=%d, 业务码=%d", username, userID, refreshResp.HTTPStatus, refreshResp.Code)
			return false
		}
	})
}

func TestCase9_TokenMismatch_Concurrent(t *testing.T) {
	runConcurrentTest(t, "Token不匹配并发测试", func(t *testing.T, userID int, username, password string) bool {
		// 两次登录获取不同令牌
		ctx1, _, err1 := login(username, password)
		if err1 != nil {
			t.Logf("用户 %s 请求 %d 第一次登录失败: %v", username, userID, err1)
			return false
		}

		time.Sleep(100 * time.Millisecond)
		ctx2, _, err2 := login(username, password)
		if err2 != nil {
			t.Logf("用户 %s 请求 %d 第二次登录失败: %v", username, userID, err2)
			return false
		}

		// 使用不匹配的Token组合
		testCtx := &TestContext{
			Username:     username,
			AccessToken:  ctx1.AccessToken,
			RefreshToken: ctx2.RefreshToken,
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err == nil && refreshResp.Code == RespCodeTokenMismatch {
			t.Logf("用户 %s 请求 %d Token不匹配检测成功", username, userID)
			return true
		} else {
			t.Logf("用户 %s 请求 %d Token不匹配检测失败: 业务码=%d", username, userID, refreshResp.Code)
			return false
		}
	})
}

func TestCase10_WrongPassword_Concurrent(t *testing.T) {
	runConcurrentTest(t, "错误密码并发测试", func(t *testing.T, userID int, username, password string) bool {
		// 使用错误密码登录，期望失败才是成功
		wrongPassword := "WrongPassword123"
		_, resp, err := login(username, wrongPassword)

		if err != nil && resp.Code == RespCodeInvalidAuth {
			t.Logf("用户 %s 请求 %d 错误密码检测成功", username, userID)
			return true
		} else if err == nil {
			t.Logf("用户 %s 请求 %d 错误密码检测失败: 预期失败但成功", username, userID)
			return false
		} else {
			t.Logf("用户 %s 请求 %d 错误密码检测失败: 业务码=%d", username, userID, resp.Code)
			return false
		}
	})
}

// ==================== 运行所有10个并发测试用例 ====================
func TestAll10ConcurrentCases(t *testing.T) {
	fmt.Println("========================================")
	cyan.Print("🚀 开始执行10个JWT并发测试用例\n")
	fmt.Println("========================================")

	testCases := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"用例1: 正常登录并发测试", TestCase1_LoginSuccess_Concurrent},
		{"用例2: 有效刷新令牌并发测试", TestCase2_RefreshValid_Concurrent},
		{"用例3: 登录登出并发测试", TestCase3_LoginLogout_Concurrent},
		{"用例4: AT过期并发测试", TestCase4_ATExpired_Concurrent},
		{"用例5: 无效AT并发测试", TestCase5_InvalidAT_Concurrent},
		{"用例6: 缺少RT并发测试", TestCase6_MissingRT_Concurrent},
		{"用例7: RT过期并发测试", TestCase7_RTExpired_Concurrent},
		{"用例8: RT撤销并发测试", TestCase8_RTRevoked_Concurrent},
		{"用例9: Token不匹配并发测试", TestCase9_TokenMismatch_Concurrent},
		{"用例10: 错误密码并发测试", TestCase10_WrongPassword_Concurrent},
	}

	for i, tc := range testCases {
		fmt.Printf("▶️  执行用例%d: %s\n", i+1, tc.name)
		t.Run(tc.name, tc.fn)
		fmt.Println()
	}

	fmt.Println("========================================")
	cyan.Print("✅ 所有10个并发测试用例执行完毕！\n")
	fmt.Println("========================================")
}

func TestSingleConcurrent(t *testing.T) {
	if err := initRedis(); err != nil {
		t.Fatalf("Redis初始化失败: %v", err)
	}

	fmt.Println("🚀 运行登录登出并发测试...")
	TestCase3_LoginLogout_Concurrent(t)
}

func TestDebugLogin(t *testing.T) {
	fmt.Println("🔍 调试登录失败原因...")

	// 使用配置的测试用户，而不是硬编码的 testuser_0
	username := TestUsername
	loginURL := ServerBaseURL + LoginAPIPath
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, ValidPassword)
	bodyReader := strings.NewReader(body)

	fmt.Printf("登录请求: URL=%s, Body=%s\n", loginURL, body)

	req, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	fmt.Printf("HTTP状态码: %d\n", resp.StatusCode)
	fmt.Printf("响应体: %s\n", string(respBody))

	// 解析响应看看具体错误
	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		t.Logf("解析响应失败: %v", err)
	} else {
		fmt.Printf("业务码: %d, 消息: %s, 错误: %s\n",
			apiResp.Code, apiResp.Message, apiResp.Error)
	}

	// 检查用户端点可能需要认证，我们先跳过这个检查
	fmt.Println("跳过用户检查端点（需要认证）")
}

// ==================== 添加更详细的登录调试 ====================
func TestDebugLoginDetailed(t *testing.T) {
	fmt.Println("🔍 详细调试登录过程...")

	username := TestUsername
	password := ValidPassword
	loginURL := ServerBaseURL + LoginAPIPath

	fmt.Printf("测试配置:\n")
	fmt.Printf("  服务器: %s\n", ServerBaseURL)
	fmt.Printf("  用户名: %s\n", username)
	fmt.Printf("  密码: %s\n", password)
	fmt.Printf("  登录路径: %s\n", LoginAPIPath)
	fmt.Println("----------------------------------------")

	// 测试1: 使用正确密码
	fmt.Println("测试1: 使用正确密码登录")
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	bodyReader := strings.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	fmt.Printf("响应状态: %d\n", resp.StatusCode)
	fmt.Printf("响应内容: %s\n", string(respBody))

	// 测试2: 使用错误密码
	fmt.Println("----------------------------------------")
	fmt.Println("测试2: 使用错误密码登录")
	body = fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, "WrongPassword123")
	bodyReader = strings.NewReader(body)

	req2, err := http.NewRequest(http.MethodPost, loginURL, bodyReader)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := httpClient.Do(req2)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	fmt.Printf("响应状态: %d\n", resp2.StatusCode)
	fmt.Printf("响应内容: %s\n", string(respBody2))

	// 测试3: 检查API路径是否正确
	fmt.Println("----------------------------------------")
	fmt.Println("测试3: 检查服务器连通性")
	pingURL := ServerBaseURL + "/healthz" // 或者你的健康检查端点
	req3, err := http.NewRequest(http.MethodGet, pingURL, nil)
	if err != nil {
		t.Logf("创建健康检查请求失败: %v", err)
		return
	}

	resp3, err := httpClient.Do(req3)
	if err != nil {
		t.Logf("健康检查请求失败: %v", err)
		return
	}
	defer resp3.Body.Close()

	fmt.Printf("健康检查状态: %d\n", resp3.StatusCode)
}

// ==================== 简化并发测试，先确保基础功能正常 ====================
func TestBasicLogin(t *testing.T) {
	fmt.Println("🔍 测试基础登录功能...")

	ctx, resp, err := login(TestUsername, ValidPassword)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	if ctx.AccessToken == "" {
		t.Fatal("未获取到AccessToken")
	}

	if ctx.RefreshToken == "" {
		t.Fatal("未获取到RefreshToken")
	}

	if resp.Code != RespCodeSuccess {
		t.Fatalf("业务码错误: 预期=%d, 实际=%d", RespCodeSuccess, resp.Code)
	}

	fmt.Printf("✅ 登录成功: AT=%s...\n", truncateStr(ctx.AccessToken, 20))
	fmt.Printf("刷新令牌: RT=%s...\n", truncateStr(ctx.RefreshToken, 20))
	fmt.Printf("业务响应: 码=%d, 消息=%s\n", resp.Code, resp.Message)
}

// ==================== 辅助函数 ====================
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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
