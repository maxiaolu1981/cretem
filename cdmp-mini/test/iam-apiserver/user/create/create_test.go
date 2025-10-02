/*
真正并发压力测试：包含Token认证的高并发用户创建
*/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8088"
	RequestTimeout = 30 * time.Second

	LoginAPIPath = "/login"
	UsersAPIPath = "/v1/users"

	RespCodeSuccess = 100001

	// 测试账号（需要先确保这个账号存在）
	TestUsername = "admin"
	TestPassword = "Admin@2021"

	// 并发配置（先调小进行调试）
	ConcurrentUsers = 10000 // 并发用户数（调试阶段调小）
	RequestsPerUser = 100   // 每用户请求数
	MaxConcurrent   = 100   // 最大并发数
	BatchSize       = 100   // 批次大小
)

// ==================== 数据结构 ====================
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// 更灵活的登录响应结构，适配不同格式
type LoginResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 使用interface{}适配不同结构
	Token   string      `json:"token,omitempty"`
}

// 具体的Token数据结构
type TokenData struct {
	Token string `json:"token"`
	User  struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

type CreateUserRequest struct {
	Metadata *UserMetadata `json:"metadata,omitempty"`
	Nickname string        `json:"nickname"`
	Password string        `json:"password"`
	Email    string        `json:"email"`
	Phone    string        `json:"phone,omitempty"`
	Status   int           `json:"status,omitempty"`
	IsAdmin  int           `json:"isAdmin,omitempty"`
}

type UserMetadata struct {
	Name string `json:"name,omitempty"`
}

type APIResponse struct {
	HTTPStatus int         `json:"-"`
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

type TestResult struct {
	UserID    int
	RequestID int
	Success   bool
	Duration  time.Duration
	Error     string
}

// ==================== 全局变量 ====================
var (
	httpClient  = createHTTPClient()
	statsMutex  sync.RWMutex
	globalToken string
	tokenMutex  sync.RWMutex
	tokenExpiry time.Time
)

// 统计变量
var (
	totalRequests int64
	successCount  int64
	failCount     int64
	totalDuration time.Duration
	errorResults  []TestResult
)

// ==================== 主测试函数 ====================
func TestUserCreate_RealConcurrent(t *testing.T) {
	// 初始化环境
	checkResourceLimits()
	setHigherFileLimit()

	width := getTerminalWidth()
	printHeader("🚀 开始带Token认证的压力测试", width)

	// 1. 首先获取认证Token（带详细调试信息）
	fmt.Printf("🔑 获取认证Token...\n")
	fmt.Printf("   登录URL: %s%s\n", ServerBaseURL, LoginAPIPath)
	fmt.Printf("   用户名: %s\n", TestUsername)

	token, err := getAuthTokenWithDebug()
	if err != nil {
		fmt.Printf("❌ 获取Token失败: %v\n", err)
		fmt.Printf("💡 建议检查:\n")
		fmt.Printf("   1. 服务是否运行在 %s\n", ServerBaseURL)
		fmt.Printf("   2. 用户 %s 是否存在\n", TestUsername)
		fmt.Printf("   3. 登录接口路径是否正确: %s\n", LoginAPIPath)
		return
	}

	tokenMutex.Lock()
	globalToken = token
	tokenExpiry = time.Now().Add(30 * time.Minute)
	tokenMutex.Unlock()

	fmt.Printf("✅ 成功获取Token: %s...\n", token[:min(20, len(token))])

	// // 2. 先测试单个请求确保Token有效
	// fmt.Printf("🧪 测试Token有效性...\n")
	// if !testTokenValidity(token) {
	// 	fmt.Printf("❌ Token测试失败，停止压力测试\n")
	// 	return
	// }
	// fmt.Printf("✅ Token测试通过\n")

	// 测试配置
	totalExpectedRequests := ConcurrentUsers * RequestsPerUser
	fmt.Printf("📊 测试配置:\n")
	fmt.Printf("  ├─ 并发用户数: %d\n", ConcurrentUsers)
	fmt.Printf("  ├─ 每用户请求数: %d\n", RequestsPerUser)
	fmt.Printf("  ├─ 总请求数: %d\n", totalExpectedRequests)
	fmt.Printf("  ├─ 最大并发数: %d\n", MaxConcurrent)
	fmt.Printf("  └─ 使用Token认证: 是\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()

	// 启动实时统计显示
	stopStats := startRealTimeStats(startTime)
	defer stopStats()

	// 执行并发测试
	executeConcurrentTestWithAuth()

	// 输出最终结果
	duration := time.Since(startTime)
	printFinalResults(duration, width)

	// 数据校验
	validateResults(width)
}

// ==================== 简化的Token解析 ====================
func getAuthTokenWithDebug() (string, error) {
	loginReq := LoginRequest{
		Username: TestUsername,
		Password: TestPassword,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("登录请求序列化失败: %v", err)
	}

	fmt.Printf("   发送登录请求...\n")
	resp, err := httpClient.Post(ServerBaseURL+LoginAPIPath, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("   响应状态码: %d\n", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取登录响应失败: %v", err)
	}

	fmt.Printf("   响应体: %s\n", string(body))

	// 直接解析为具体结构
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken  string `json:"access_token"`
			Expire       string `json:"expire"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v", err)
	}

	fmt.Printf("   响应Code: %d, Message: %s\n", response.Code, response.Message)

	if response.Code != RespCodeSuccess {
		return "", fmt.Errorf("登录失败: %s", response.Message)
	}

	if response.Data.AccessToken == "" {
		return "", fmt.Errorf("access_token为空")
	}

	fmt.Printf("   ✅ 成功获取access_token，长度: %d\n", len(response.Data.AccessToken))
	return response.Data.AccessToken, nil
}

func getValidToken() string {
	tokenMutex.RLock()
	token := globalToken
	expiry := tokenExpiry
	tokenMutex.RUnlock()

	if token == "" || time.Now().Add(5*time.Minute).After(expiry) {
		newToken, err := getAuthTokenWithDebug()
		if err != nil {
			fmt.Printf("⚠️  Token刷新失败: %v\n", err)
			return token
		}

		tokenMutex.Lock()
		globalToken = newToken
		tokenExpiry = time.Now().Add(30 * time.Minute)
		tokenMutex.Unlock()

		return newToken
	}

	return token
}

// ==================== 核心并发逻辑 ====================
func executeConcurrentTestWithAuth() {
	semaphore := make(chan struct{}, MaxConcurrent)
	var wg sync.WaitGroup

	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	for batch := 0; batch < totalBatches; batch++ {
		batchStart := batch * BatchSize
		batchEnd := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("🔄 处理批次 %d/%d: 用户 %d-%d\n",
			batch+1, totalBatches, batchStart, batchEnd-1)

		var batchWg sync.WaitGroup
		for userID := batchStart; userID < batchEnd; userID++ {
			batchWg.Add(1)
			go func(uid int) {
				defer batchWg.Done()
				sendUserRequestsWithAuth(uid, semaphore, &wg)
			}(userID)
		}
		batchWg.Wait()

		if batch < totalBatches-1 {
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		}
	}

	wg.Wait()
}

func sendUserRequestsWithAuth(userID int, semaphore chan struct{}, wg *sync.WaitGroup) {
	var userWg sync.WaitGroup

	for requestID := 0; requestID < RequestsPerUser; requestID++ {
		wg.Add(1)
		userWg.Add(1)
		semaphore <- struct{}{}

		go func(uid, rid int) {
			defer wg.Done()
			defer userWg.Done()
			defer func() { <-semaphore }()

			sendSingleRequestWithAuth(uid, rid)
		}(userID, requestID)

		time.Sleep(50 * time.Microsecond)
	}

	userWg.Wait()
}

func sendSingleRequestWithAuth(userID, requestID int) {
	start := time.Now()

	// 获取有效Token
	token := getValidToken()
	if token == "" {
		recordResult(userID, requestID, false, time.Since(start), "Token获取失败")
		return
	}

	// 准备请求数据
	userReq := CreateUserRequest{
		Metadata: &UserMetadata{
			Name: generateUniqueUsername(userID),
		},
		Nickname: fmt.Sprintf("测试用户%d", userID),
		Password: "Test@123456",
		Email:    fmt.Sprintf("testuser%d@example.com", userID),
		Phone:    fmt.Sprintf("138%08d", userID),
		Status:   1,
		IsAdmin:  0,
	}

	jsonData, err := json.Marshal(userReq)
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), "JSON序列化失败")
		return
	}

	// 创建带Token的请求
	req, err := http.NewRequest("POST", ServerBaseURL+UsersAPIPath, bytes.NewReader(jsonData))
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), "创建请求失败")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), fmt.Sprintf("请求发送失败: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), "响应读取失败")
		return
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		recordResult(userID, requestID, false, time.Since(start), "响应解析失败")
		return
	}
	apiResp.HTTPStatus = resp.StatusCode

	duration := time.Since(start)
	success := resp.StatusCode == http.StatusCreated && apiResp.Code == RespCodeSuccess

	if success {
		recordResult(userID, requestID, true, duration, "")
	} else {
		if resp.StatusCode == http.StatusUnauthorized {
			tokenMutex.Lock()
			globalToken = ""
			tokenMutex.Unlock()
		}

		recordResult(userID, requestID, false, duration,
			fmt.Sprintf("HTTP=%d, Code=%d, Msg=%s", resp.StatusCode, apiResp.Code, apiResp.Message))
	}
}

// ==================== 以下工具函数保持不变 ====================
func recordResult(userID, requestID int, success bool, duration time.Duration, errorMsg string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	totalRequests++

	if success {
		successCount++
		totalDuration += duration
	} else {
		failCount++
		errorResults = append(errorResults, TestResult{
			UserID:    userID,
			RequestID: requestID,
			Success:   success,
			Duration:  duration,
			Error:     errorMsg,
		})
	}
}

func startRealTimeStats(startTime time.Time) func() {
	ticker := time.NewTicker(500 * time.Millisecond)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				statsMutex.RLock()
				currentTotal := totalRequests
				currentSuccess := successCount
				currentFail := failCount
				statsMutex.RUnlock()

				if currentTotal == 0 {
					continue
				}

				duration := time.Since(startTime)
				qps := float64(currentTotal) / duration.Seconds()
				successRate := float64(currentSuccess) / float64(currentTotal) * 100

				fmt.Printf("\r📈 实时统计: 请求=%d, 成功=%d, 失败=%d, QPS=%.1f, 成功率=%.1f%%, 耗时=%v",
					currentTotal, currentSuccess, currentFail, qps, successRate, duration.Round(time.Second))

			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		done <- true
	}
}

func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:          10000,
			MaxIdleConnsPerHost:   10000,
			MaxConnsPerHost:       10000,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			DisableKeepAlives:     false,
		},
	}
}

func generateUniqueUsername(userID int) string {
	timestamp := time.Now().UnixNano() % 1000000
	random := rand.IntN(1000000)
	return fmt.Sprintf("user_%d_%d_%d", userID, timestamp, random)
}

func getTerminalWidth() int {
	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}
	return width
}

func printHeader(title string, width int) {
	fmt.Printf("\n%s\n", strings.Repeat("═", width))
	fmt.Printf("%s\n", title)
	fmt.Printf("%s\n", strings.Repeat("═", width))
}

func printFinalResults(duration time.Duration, width int) {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	fmt.Printf("\n\n🎯 压力测试完成!\n")
	fmt.Printf("%s\n", strings.Repeat("═", width))

	successRate := float64(successCount) / float64(totalRequests) * 100
	qps := float64(totalRequests) / duration.Seconds()

	fmt.Printf("📊 性能统计:\n")
	fmt.Printf("  ├─ 总请求数: %d\n", totalRequests)
	fmt.Printf("  ├─ 成功数: %d\n", successCount)
	fmt.Printf("  ├─ 失败数: %d\n", failCount)
	fmt.Printf("  ├─ 成功率: %.2f%%\n", successRate)
	fmt.Printf("  ├─ 总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("  ├─ 平均QPS: %.1f\n", qps)

	if successCount > 0 {
		avgDuration := totalDuration / time.Duration(successCount)
		fmt.Printf("  └─ 平均响应时间: %v\n", avgDuration.Round(time.Millisecond))
	}

	if len(errorResults) > 0 {
		fmt.Printf("\n🔍 错误分析 (前10个):\n")
		displayErrors := min(10, len(errorResults))
		for i := 0; i < displayErrors; i++ {
			err := errorResults[i]
			fmt.Printf("  %d. 用户%d-请求%d: %s (耗时: %v)\n",
				i+1, err.UserID, err.RequestID, err.Error, err.Duration.Round(time.Millisecond))
		}
	}

	fmt.Printf("%s\n", strings.Repeat("═", width))
}

func validateResults(width int) {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	expected := ConcurrentUsers * RequestsPerUser
	if int(totalRequests) != expected {
		fmt.Printf("⚠️  统计警告: 实际请求数(%d) != 预期请求数(%d)\n", totalRequests, expected)
	}
}

func checkResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		fmt.Printf("📁 文件描述符限制: Soft=%d, Hard=%d\n", rLimit.Cur, rLimit.Max)
	}
}

func setHigherFileLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		if rLimit.Cur < 10000 && rLimit.Max >= 10000 {
			rLimit.Cur = 10000
			if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
				fmt.Printf("✅ 文件描述符限制已设置为: %d\n", rLimit.Cur)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
