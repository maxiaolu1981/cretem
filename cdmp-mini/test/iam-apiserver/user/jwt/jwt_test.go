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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	redisV8 "github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
	"golang.org/x/term"
)

var serverBaseURL = ServerBaseURL

var testUsers = []struct {
	username string
	password string
}{
	{"admin", "Admin@2021"},

	// ... 添加更多用户
}

const (
	ExpectedHTTPSuccess = 200 // 期望的HTTP成功状态码
	// 业务成功码根据不同的测试用例动态传入
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8080"
	RequestTimeout = 10 * time.Second

	LoginAPIPath   = "/login"
	RefreshAPIPath = "/refresh"
	LogoutAPIPath  = "/logout"

	RedisAddr     = "192.168.10.14:6379"
	RedisPassword = ""
	RedisDB       = 0

	TestUsername    = "admin"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022"

	JWTSigningKey = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo"
	JWTAlgorithm  = "HS256"

	RTRedisPrefix = "genericapiserver:auth:refresh_token:"

	RespCodeSuccess       = 100001
	RespCodeRTRequired    = 110004
	RespCodeRTRevoked     = 100203
	RespCodeATExpired     = 100203
	RespCodeInvalidAT     = 100208
	RespCodeRTExpired     = 100203
	RespCodeTokenMismatch = 100212
	RespCodeInvalidAuth   = 110004

	//ConcurrentUsers = 1
	//RequestsPerUser = 1

	ConcurrentUsers = 10
	RequestsPerUser = 10

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

type TestResult struct {
	User         string
	RequestID    int
	Success      bool
	ExpectedHTTP int
	ExpectedBiz  int
	ActualHTTP   int
	ActualBiz    int
	Message      string
}

// ==================== 全局变量 ====================
var (
	httpClient  = &http.Client{Timeout: RequestTimeout}
	redisClient *redisV8.Client
	cyan        = color.New(color.FgCyan)
)

func TestMain(m *testing.M) {
	if override := os.Getenv("IAM_APISERVER_BASEURL"); override != "" {
		serverBaseURL = override
	}
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] 设置 IAM_APISERVER_E2E=1 才会执行 IAM API Server 集成测试")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// ==================== 修复的响应码统计函数 ====================
func printResponseCodeSummary(results []TestResult) {
	httpCodeCount := make(map[int]int)
	bizCodeCount := make(map[int]int)

	for _, result := range results {
		httpCodeCount[result.ActualHTTP]++
		bizCodeCount[result.ActualBiz]++
	}

	fmt.Printf("\n📋 响应码统计:\n")
	fmt.Printf("   HTTP状态码分布:\n")
	for code, count := range httpCodeCount {
		fmt.Printf("     %d: %d次\n", code, count)
	}

	fmt.Printf("   业务码分布:\n")
	for code, count := range bizCodeCount {
		fmt.Printf("     %d: %d次\n", code, count)
	}
}

// ==================== 修复的详细结果表格函数 ====================
func printDetailedResultsTable(results []TestResult) {
	if len(results) == 0 {
		return
	}

	fmt.Printf("\n📋 详细结果对比:\n")
	fmt.Printf("┌─────────┬──────────┬────────┬──────────────┬──────────────┬────────────────────┐\n")
	fmt.Printf("│  用户   │  请求ID  │  结果  │  HTTP状态码  │   业务码     │       消息        │\n")
	fmt.Printf("│         │          │        │  预期/实际   │  预期/实际   │                    │\n")
	fmt.Printf("├─────────┼──────────┼────────┼──────────────┼──────────────┼────────────────────┤\n")

	for _, result := range results {
		user := truncateStr(result.User, 6)
		status := "✅"
		if !result.Success {
			status = "❌"
		}
		message := truncateStr(result.Message, 18)

		httpCompare := fmt.Sprintf("%d/%d", result.ExpectedHTTP, result.ActualHTTP)
		bizCompare := fmt.Sprintf("%d/%d", result.ExpectedBiz, result.ActualBiz)

		fmt.Printf("│ %-7s │ %8d │ %-6s │ %-12s │ %-12s │ %-18s │\n",
			user, result.RequestID, status, httpCompare, bizCompare, message)
	}
	fmt.Printf("└─────────┴──────────┴────────┴──────────────┴──────────────┴────────────────────┘\n")
}

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
	apiResp.HTTPStatus = resp.StatusCode
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

func runConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {
	// 获取终端宽度
	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	// 清屏并设置显示区域
	fmt.Print("\033[2J")   // 清屏
	fmt.Print("\033[1;1H") // 光标移动到左上角

	// 显示顶部标题
	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("🚀 开始并发测试: %s\n", testName)
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 预留进度显示区域（第4-6行）
	fmt.Printf("\033[4;1H") // 移动到第4行
	fmt.Printf("进度显示区域...\n\n\n")

	// 预留统计结果显示区域（从第7行开始）
	fmt.Printf("\033[7;1H") // 移动到第7行
	fmt.Printf("%s\n", strings.Repeat("─", width))
	fmt.Printf("📊 实时统计:\n")
	fmt.Printf("   ✅ 成功请求: 0\n")
	fmt.Printf("   ❌ 失败请求: 0\n")
	fmt.Printf("   📈 成功率: 0.0%%\n")
	fmt.Printf("   ⏱️  当前耗时: 0ms\n")
	fmt.Printf("   🚀 实时QPS: 0.0\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failCount := 0
	var testResults []TestResult

	// 创建进度通道
	progress := make(chan string, ConcurrentUsers*RequestsPerUser)
	done := make(chan bool)

	// 启动进度显示器
	go func() {
		line := 4 // 从第4行开始显示进度
		for msg := range progress {
			fmt.Printf("\033[%d;1H", line) // 移动到指定行
			fmt.Printf("\033[K")           // 清除行
			fmt.Printf("   %s", msg)
			line++
			if line > 6 { // 保持在4-6行范围内
				line = 4
			}
		}
		done <- true
	}()

	// 启动统计信息更新器
	statsTicker := time.NewTicker(100 * time.Millisecond)
	defer statsTicker.Stop()

	go func() {
		for range statsTicker.C {
			mu.Lock()
			currentSuccess := successCount
			currentFail := failCount
			currentDuration := time.Since(startTime)
			totalRequests := currentSuccess + currentFail
			mu.Unlock()

			if totalRequests > 0 {
				// 更新统计信息区域（第8-13行）
				fmt.Printf("\033[8;1H") // 移动到第8行
				fmt.Printf("\033[K")    // 清除行
				fmt.Printf("   ✅ 成功请求: %d\n", currentSuccess)
				fmt.Printf("\033[9;1H\033[K")
				fmt.Printf("   ❌ 失败请求: %d\n", currentFail)
				fmt.Printf("\033[10;1H\033[K")
				fmt.Printf("   📈 成功率: %.1f%%\n", float64(currentSuccess)/float64(totalRequests)*100)
				fmt.Printf("\033[11;1H\033[K")
				fmt.Printf("   ⏱️  当前耗时: %v\n", currentDuration.Round(time.Millisecond))
				fmt.Printf("\033[12;1H\033[K")
				fmt.Printf("   🚀 实时QPS: %.1f\n", float64(totalRequests)/currentDuration.Seconds())
				fmt.Printf("\033[13;1H\033[K")
				fmt.Printf("%s", strings.Repeat("─", width))
			}
		}
	}()

	for i := 0; i < ConcurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

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

				// 显示进度
				progress <- fmt.Sprintf("🟡 [用户%s] 请求 %d/%d 开始...", username, requestID, ConcurrentUsers*RequestsPerUser)

				// 调用测试函数
				success, resp, expectedHTTP, expectedBiz := testFunc(t, userID, username, password)

				mu.Lock()
				if success {
					successCount++
					progress <- fmt.Sprintf("🟢 [用户%s] 请求 %d 成功", username, requestID)
				} else {
					failCount++
					progress <- fmt.Sprintf("🔴 [用户%s] 请求 %d 失败", username, requestID)
				}

				// 记录测试结果详情
				if resp != nil {
					testResults = append(testResults, TestResult{
						User:         username,
						RequestID:    requestID,
						Success:      success,
						ExpectedHTTP: expectedHTTP,
						ExpectedBiz:  expectedBiz,
						ActualHTTP:   resp.HTTPStatus,
						ActualBiz:    resp.Code,
						Message:      resp.Message,
					})
				} else {
					testResults = append(testResults, TestResult{
						User:         username,
						RequestID:    requestID,
						Success:      success,
						ExpectedHTTP: expectedHTTP,
						ExpectedBiz:  expectedBiz,
						ActualHTTP:   0,
						ActualBiz:    0,
						Message:      "无响应",
					})
				}
				mu.Unlock()

				time.Sleep(50 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(progress)
	<-done
	statsTicker.Stop()

	duration := time.Since(startTime)
	totalRequests := ConcurrentUsers * RequestsPerUser

	// 显示最终结果在统计区域
	fmt.Printf("\033[8;1H\033[K")
	fmt.Printf("   ✅ 成功请求: %d\n", successCount)
	fmt.Printf("\033[9;1H\033[K")
	fmt.Printf("   ❌ 失败请求: %d\n", failCount)
	fmt.Printf("\033[10;1H\033[K")
	fmt.Printf("   📈 成功率: %.1f%%\n", float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("\033[11;1H\033[K")
	fmt.Printf("   ⏱️  总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("\033[12;1H\033[K")
	fmt.Printf("   🚀 最终QPS: %.1f\n", float64(totalRequests)/duration.Seconds())
	fmt.Printf("\033[13;1H\033[K")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 输出响应码统计和详细结果表格
	fmt.Printf("\033[15;1H") // 移动到第15行
	printResponseCodeSummary(testResults)
	printDetailedResultsTable(testResults)

	fmt.Printf("\033[30;1H") // 移动到屏幕底部
	fmt.Printf("%s\n", strings.Repeat("═", width))

	t.Logf("测试完成: %s, 成功率: %.1f%%, 耗时: %v", testName, float64(successCount)/float64(totalRequests)*100, duration)
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

// ==================== 修复所有测试用例的函数签名 ====================

// ==================== 修改所有测试用例函数签名 ====================

// ==================== 修改所有10个测试用例函数签名 ====================

func TestCase1_LoginSuccess_Concurrent(t *testing.T) {
	runConcurrentTest(t, "正常登录并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {

		ctx, resp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, resp, http.StatusOK, RespCodeSuccess
		}

		success := ctx.AccessToken != "" &&
			strings.Count(ctx.AccessToken, ".") == 2 &&
			resp.Code == RespCodeSuccess

		return success, resp, http.StatusOK, RespCodeSuccess
	})
}

func TestCase2_RefreshValid_Concurrent(t *testing.T) {
	runConcurrentTest(t, "有效刷新令牌并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		ctx, _, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, nil, http.StatusOK, RespCodeSuccess
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 刷新请求失败: %v", username, userID, err)
			return false, refreshResp, http.StatusOK, RespCodeSuccess
		}

		success := refreshResp.Code == RespCodeSuccess
		return success, refreshResp, http.StatusOK, RespCodeSuccess
	})
}

func TestCase3_LoginLogout_Concurrent(t *testing.T) {
	runConcurrentTest(t, "登录登出并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		// 登录
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusOK, RespCodeSuccess
		}

		// 登出
		logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(logoutBody)
		logoutResp, err := sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 登出请求失败: %v", username, userID, err)
			return false, logoutResp, http.StatusOK, RespCodeSuccess
		}

		success := logoutResp.Code == RespCodeSuccess
		return success, logoutResp, http.StatusOK, RespCodeSuccess
	})
}

func TestCase4_ATExpired_Concurrent(t *testing.T) {
	runConcurrentTest(t, "AT过期并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusOK, RespCodeSuccess
		}

		// 修改AT为过期状态但保留声明信息
		expiredAT, err := modifyTokenToExpired(ctx.AccessToken)
		if err != nil {
			t.Logf("用户 %s 请求 %d 生成过期AT失败: %v", username, userID, err)
			return false, nil, http.StatusOK, RespCodeSuccess
		}

		testCtx := &TestContext{
			Username:     username,
			AccessToken:  expiredAT,
			RefreshToken: ctx.RefreshToken,
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 过期AT刷新失败: %v", username, userID, err)
			return false, refreshResp, http.StatusOK, RespCodeSuccess
		}

		// 期望成功刷新（如果系统实现正确）
		success := refreshResp.Code == RespCodeSuccess
		return success, refreshResp, http.StatusOK, RespCodeSuccess
	})
}

func TestCase5_InvalidAT_Concurrent(t *testing.T) {
	runConcurrentTest(t, "无效AT并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusUnauthorized, RespCodeInvalidAT
		}

		testCtx := &TestContext{
			Username:     username,
			AccessToken:  "invalid.token.format",
			RefreshToken: ctx.RefreshToken,
		}

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, testCtx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(testCtx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 无效AT检测失败: %v", username, userID, err)
			return false, refreshResp, http.StatusUnauthorized, RespCodeInvalidAT
		}

		success := refreshResp.Code == RespCodeInvalidAT
		return success, refreshResp, http.StatusUnauthorized, RespCodeInvalidAT
	})
}

func TestCase6_MissingRT_Concurrent(t *testing.T) {
	runConcurrentTest(t, "缺少RT并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusBadRequest, RespCodeRTRequired
		}

		refreshBody := `{"refresh_token": ""}`
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 缺少RT检测失败: %v", username, userID, err)
			return false, refreshResp, http.StatusBadRequest, RespCodeRTRequired
		}

		success := refreshResp.Code == RespCodeRTRequired
		return success, refreshResp, http.StatusBadRequest, RespCodeRTRequired
	})
}

func TestCase7_RTExpired_Concurrent(t *testing.T) {
	if err := initRedis(); err != nil {
		t.Fatalf("Redis初始化失败: %v", err)
		return
	}

	runConcurrentTest(t, "RT过期并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusUnauthorized, RespCodeRTExpired
		}

		// 设置RT过期
		rtKey := fmt.Sprintf("%s%s", RTRedisPrefix, ctx.RefreshToken)
		redisCtx := context.Background()
		redisClient.Expire(redisCtx, rtKey, 1*time.Second)
		time.Sleep(2 * time.Second)

		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 过期RT检测失败: %v", username, userID, err)
			return false, refreshResp, http.StatusUnauthorized, RespCodeRTExpired
		}

		success := refreshResp.Code == RespCodeRTExpired
		return success, refreshResp, http.StatusUnauthorized, RespCodeRTExpired
	})
}

func TestCase8_RTRevoked_Concurrent(t *testing.T) {
	runConcurrentTest(t, "RT撤销并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		// 登录并立即注销
		ctx, loginResp, err := login(username, password)
		if err != nil {
			t.Logf("用户 %s 请求 %d 登录失败: %v", username, userID, err)
			return false, loginResp, http.StatusUnauthorized, RespCodeRTRevoked
		}

		// 注销使RT失效
		logoutBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader := strings.NewReader(logoutBody)
		logoutResp, err := sendTokenRequest(ctx, http.MethodPost, LogoutAPIPath, bodyReader)

		if err != nil || logoutResp.Code != RespCodeSuccess {
			t.Logf("用户 %s 请求 %d 注销失败", username, userID)
			return false, logoutResp, http.StatusUnauthorized, RespCodeRTRevoked
		}

		time.Sleep(100 * time.Millisecond)

		// 尝试使用已撤销的RT刷新
		refreshBody := fmt.Sprintf(`{"refresh_token": "%s"}`, ctx.RefreshToken)
		bodyReader = strings.NewReader(refreshBody)
		refreshResp, err := sendTokenRequest(ctx, http.MethodPost, RefreshAPIPath, bodyReader)

		if err != nil {
			t.Logf("用户 %s 请求 %d 撤销RT检测失败: %v", username, userID, err)
			return false, refreshResp, http.StatusUnauthorized, RespCodeRTRevoked
		}

		success := refreshResp.Code == RespCodeRTRevoked || refreshResp.HTTPStatus == http.StatusUnauthorized
		return success, refreshResp, http.StatusUnauthorized, RespCodeRTRevoked
	})
}

func TestCase9_TokenMismatch_Concurrent(t *testing.T) {
	runConcurrentTest(t, "Token不匹配并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		// 第一次登录
		ctx1, loginResp1, err1 := login(username, password)
		if err1 != nil || ctx1 == nil {
			t.Logf("用户 %s 请求 %d 第一次登录失败: %v", username, userID, err1)
			return false, loginResp1, http.StatusUnauthorized, RespCodeTokenMismatch
		}

		// 第二次登录
		ctx2, loginResp2, err2 := login(username, password)
		if err2 != nil || ctx2 == nil {
			t.Logf("用户 %s 请求 %d 第二次登录失败: %v", username, userID, err2)
			return false, loginResp2, http.StatusUnauthorized, RespCodeTokenMismatch
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

		if err != nil || refreshResp == nil {
			t.Logf("用户 %s 请求 %d Token不匹配检测失败: %v", username, userID, err)
			// 创建默认的错误响应
			errorResp := &APIResponse{
				HTTPStatus: http.StatusInternalServerError,
				Code:       RespCodeTokenMismatch,
				Message:    "请求失败",
			}
			return false, errorResp, http.StatusUnauthorized, RespCodeTokenMismatch
		}

		success := refreshResp.Code == RespCodeTokenMismatch
		return success, refreshResp, http.StatusUnauthorized, RespCodeTokenMismatch
	})
}

// extractSessionID 使用jwt库解析token提取session_id
func extractSessionID(tokenString string) (string, error) {
	if tokenString == "" {
		return "", errors.New("空token")
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return "", errors.New("无效的JWT格式")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64解码失败: %v", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("JSON解析失败: %v", err)
	}

	sessionID, ok := claims["session_id"].(string)
	if !ok || sessionID == "" {
		return "", errors.New("缺少session_id")
	}

	return sessionID, nil
}

// 简化的自定义设备登录函数
func loginWithCustomDevice(username, password, deviceID string) (*TestContext, *APIResponse, error) {
	loginURL := ServerBaseURL + LoginAPIPath
	loginData := map[string]string{
		"username": username,
		"password": password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodPost, loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", fmt.Sprintf("test-%d", time.Now().UnixNano()))
	req.Header.Set("X-Device-ID", deviceID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, nil, err
	}

	apiResp.HTTPStatus = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf("登录失败: %s", apiResp.Message)
	}

	// 解析token数据
	var tokenData struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &tokenData); err != nil {
		return nil, &apiResp, err
	}

	testCtx := &TestContext{
		Username:     username,
		AccessToken:  tokenData.Data.AccessToken,
		RefreshToken: tokenData.Data.RefreshToken,
	}

	return testCtx, &apiResp, nil
}

func TestCase10_WrongPassword_Concurrent(t *testing.T) {
	runConcurrentTest(t, "错误密码并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		// 使用错误密码登录，期望失败才是成功
		wrongPassword := "WrongPassword123"
		_, resp, err := login(username, wrongPassword)

		if err != nil {
			t.Logf("用户 %s 请求 %d 错误密码检测失败: %v", username, userID, err)
			return true, resp, http.StatusBadRequest, RespCodeInvalidAuth
		}

		success := resp.Code == RespCodeInvalidAuth
		return success, resp, http.StatusBadRequest, RespCodeInvalidAuth
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

// 修改令牌为过期状态，但保留所有原始声明信息
func modifyTokenToExpired(originalAT string) (string, error) {
	// 解析原始AT但不验证签名（因为我们只是要获取claims）
	parser := jwt.Parser{}
	token, _, err := parser.ParseUnverified(originalAT, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("解析原始AT失败: %w", err)
	}

	// 获取原始claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("无法获取JWT声明")
	}

	// 修改过期时间，但保留所有其他声明
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix() // 设置为1小时前过期
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix() // 设置为2小时前签发
	// 保留所有其他重要声明：sub, user_id, jti, session_id, role等

	// 使用相同的签名方法重新签名
	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return newToken.SignedString([]byte(JWTSigningKey))
}
