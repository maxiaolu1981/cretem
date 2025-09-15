package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	redisV8 "github.com/go-redis/redis/v8"
	"golang.org/x/term"
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8080"
	RequestTimeout = 10 * time.Second

	LoginAPIPath = "/login"
	UsersAPIPath = "/v1/users"

	TestUsername    = "admin"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022"

	RespCodeSuccess    = 100001
	RespCodeValidation = 100400
	RespCodeConflict   = 100409

	ConcurrentUsers = 1                     // 增加并发用户数
	RequestsPerUser = 1                     // 每个用户的请求数
	RequestInterval = 50 * time.Millisecond // 请求间隔
)

// ==================== 数据结构 ====================
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
	Duration     time.Duration
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

// ==================== 全局变量 ====================
var (
	httpClient  = &http.Client{Timeout: RequestTimeout}
	redisClient *redisV8.Client
)

// ==================== 主测试函数 ====================
func TestMain(m *testing.M) {
	fmt.Println("初始化测试环境...")

	// 运行测试
	code := m.Run()
	os.Exit(code)
}

func TestCase_CreateUserSuccess_Concurrent(t *testing.T) {
	runConcurrentTest(t, "创建用户成功并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		start := time.Now()

		// 首先登录获取token
		ctx, loginResp, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Logf("管理员 %s 登录失败: %v", TestUsername, err)
			return false, loginResp, http.StatusOK, RespCodeSuccess
		}

		// 构建用户创建请求
		userReq := CreateUserRequest{
			Metadata: &UserMetadata{
				Name: generateValidUserName(userID),
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
			t.Logf("用户请求 %d JSON序列化失败: %v", userID, err)
			return false, nil, http.StatusCreated, RespCodeSuccess
		}

		// 发送创建用户请求
		createResp, err := sendTokenRequest(ctx, http.MethodPost, UsersAPIPath, bytes.NewReader(jsonData))
		if err != nil {
			t.Logf("用户请求 %d 创建请求失败: %v", userID, err)
			return false, createResp, http.StatusCreated, RespCodeSuccess
		}

		// 验证响应
		success := createResp.HTTPStatus == http.StatusCreated && createResp.Code == RespCodeSuccess

		duration := time.Since(start)
		if !success {
			t.Logf("用户请求 %d 创建失败: HTTP=%d, Code=%d, Message=%s, 耗时: %v",
				userID, createResp.HTTPStatus, createResp.Code, createResp.Message, duration)
		} else {
			t.Logf("用户请求 %d 创建成功, 耗时: %v", userID, duration)
		}

		return success, createResp, http.StatusCreated, RespCodeSuccess
	})
}

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
		return nil, &apiResp, fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	tokenData, ok := apiResp.Data.(map[string]interface{})
	if !ok {
		return nil, &apiResp, fmt.Errorf("响应格式错误")
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	userID, _ := tokenData["user_id"].(string)

	return &TestContext{
		Username:     username,
		Userid:       userID,
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

	if ctx != nil && ctx.AccessToken != "" {
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

	return &apiResp, nil // 修复这里：apiResponse → apiResp
}

func generateValidUserName(userID int) string {
	timestamp := time.Now().UnixNano() % 10000
	return fmt.Sprintf("user_%d_%d", userID, timestamp)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ==================== 并发测试框架 ====================
func runConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {
	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	// 清屏并定位到左上角
	fmt.Print("\033[2J")
	fmt.Print("\033[1;1H")

	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("🚀 开始并发测试: %s\n", testName)
	fmt.Printf("📊 并发数: %d, 总请求数: %d\n", ConcurrentUsers, ConcurrentUsers*RequestsPerUser)
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failCount := 0
	totalDuration := time.Duration(0)
	var testResults []TestResult

	progress := make(chan string, ConcurrentUsers*RequestsPerUser)
	done := make(chan bool)

	// 进度显示协程
	go func() {
		line := 5
		for msg := range progress {
			fmt.Printf("\033[%d;1H", line)
			fmt.Printf("\033[K") // 清除行
			fmt.Printf("   %s", msg)
			line++
			if line > 8 {
				line = 5
			}
		}
		done <- true
	}()

	// 统计信息显示协程
	statsTicker := time.NewTicker(200 * time.Millisecond)
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
				fmt.Printf("\033[10;1H\033[K")
				fmt.Printf("   ✅ 成功请求: %d\n", currentSuccess)
				fmt.Printf("\033[11;1H\033[K")
				fmt.Printf("   ❌ 失败请求: %d\n", currentFail)
				fmt.Printf("\033[12;1H\033[K")
				fmt.Printf("   📈 成功率: %.1f%%\n", float64(currentSuccess)/float64(totalRequests)*100)
				fmt.Printf("\033[13;1H\033[K")
				fmt.Printf("   ⏱️  当前耗时: %v\n", currentDuration.Round(time.Millisecond))
				fmt.Printf("\033[14;1H\033[K")
				fmt.Printf("   🚀 实时QPS: %.1f\n", float64(totalRequests)/currentDuration.Seconds())
				if totalDuration > 0 && successCount > 0 {
					fmt.Printf("\033[15;1H\033[K")
					fmt.Printf("   ⚡ 平均耗时: %v\n", totalDuration/time.Duration(successCount))
				}
				fmt.Printf("\033[16;1H\033[K")
				fmt.Printf("%s", strings.Repeat("─", width))
			}
		}
	}()

	// 启动并发测试
	for i := 0; i < ConcurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			username := TestUsername
			password := ValidPassword

			for j := 0; j < RequestsPerUser; j++ {
				requestID := userID*RequestsPerUser + j + 1
				progress <- fmt.Sprintf("🟡 [用户%s] 请求 %d/%d 开始...", username, requestID, ConcurrentUsers*RequestsPerUser)

				start := time.Now()
				success, resp, expectedHTTP, expectedBiz := testFunc(t, userID, username, password)
				duration := time.Since(start)

				mu.Lock()
				if success {
					successCount++
					totalDuration += duration
					progress <- fmt.Sprintf("🟢 [用户%s] 请求 %d 成功 (耗时: %v)", username, requestID, duration)
				} else {
					failCount++
					progress <- fmt.Sprintf("🔴 [用户%s] 请求 %d 失败 (耗时: %v)", username, requestID, duration)
				}

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
						Duration:     duration,
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
						Duration:     duration,
					})
				}
				mu.Unlock()

				time.Sleep(RequestInterval)
			}
		}(i)
	}

	wg.Wait()
	close(progress)
	<-done
	statsTicker.Stop()

	// 输出最终结果
	duration := time.Since(startTime)
	totalRequests := ConcurrentUsers * RequestsPerUser

	fmt.Printf("\033[18;1H\033[K")
	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("📊 测试完成!\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))
	fmt.Printf("   ✅ 总成功数: %d/%d (%.1f%%)\n", successCount, totalRequests, float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("   ❌ 总失败数: %d/%d (%.1f%%)\n", failCount, totalRequests, float64(failCount)/float64(totalRequests)*100)
	fmt.Printf("   ⏱️  总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("   🚀 平均QPS: %.1f\n", float64(totalRequests)/duration.Seconds())
	if successCount > 0 {
		fmt.Printf("   ⚡ 平均响应时间: %v\n", totalDuration/time.Duration(successCount))
	}
	fmt.Printf("%s\n", strings.Repeat("═", width))

	// 输出详细错误信息（如果有）
	if failCount > 0 {
		fmt.Printf("\n📋 错误详情:\n")
		for _, result := range testResults {
			if !result.Success {
				fmt.Printf("   🔴 请求 %d: HTTP %d (期望 %d), 业务码 %d (期望 %d)\n",
					result.RequestID, result.ActualHTTP, result.ExpectedHTTP,
					result.ActualBiz, result.ExpectedBiz)
				fmt.Printf("       消息: %s\n", truncateStr(result.Message, 100))
			}
		}
	}
}
