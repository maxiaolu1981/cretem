/*
定义业务：先明确 "单个用户创建请求" 要做什么（登录→发请求→验证结果）；
拆分批次：把总用户拆成多批，每批执行完后释放资源，避免系统过载；
并发执行：每批启动多个协程模拟用户，同时用辅助协程实时显示进度和性能；
统计结果：每批结束后打印成功率、QPS、平均响应时间，形成完整测试报告。
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

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"golang.org/x/term"
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8088"
	RequestTimeout = 10 * time.Second

	LoginAPIPath = "/login"
	UsersAPIPath = "/v1/users"

	TestUsername    = "admin"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022"

	RespCodeSuccess    = 100001
	RespCodeValidation = 100400
	RespCodeConflict   = 100409

	ConcurrentUsers = 1000
	RequestsPerUser = 10
	RequestInterval = 5 * time.Millisecond
	BatchSize       = 50 // 减小批次大小，避免资源压力
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
	httpClient = createHTTPClient()
	mu         sync.Mutex
)

var (
	// 统计变量
	// 1. 总发送请求数（尝试发送的总数）
	TotalSentRequests = 0
	// 2. 服务器接收并返回的请求数（有响应）
	TotalServerReceivedAndReturned = 0
	// 3. 服务器返回失败数（有响应但不符合预期）
	TotalserverReturnedFail = 0
	// 4. 发送失败数（无响应，请求没到服务器）
	TotalSendFailNoResponse = 0
	TotalsuccessCount       = 0
	TotalErrTestResults     = []TestResult{}
)

func TestCase_CreateUserSuccess_Concurrent(t *testing.T) {

	// 使用新的测试函数
	runBatchConcurrentTest(t, "创建用户成功并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
		// 这个函数不会被直接调用，保持签名兼容
		return false, nil, http.StatusCreated, RespCodeSuccess
	})

	// 数据校验
	if TotalSentRequests != TotalsuccessCount+TotalserverReturnedFail+TotalSendFailNoResponse {
		fmt.Printf("   ⚠️  统计校验警告：数据不匹配！总发送数=%d，正确数+返回失败数+发送失败数=%d\n",
			TotalSentRequests, TotalsuccessCount+TotalserverReturnedFail+TotalSendFailNoResponse)
	}
	if TotalServerReceivedAndReturned != TotalsuccessCount+TotalserverReturnedFail {
		fmt.Printf("   ⚠️  统计校验警告：服务器接收数不匹配！接收数=%d，正确数+返回失败数=%d\n",
			TotalServerReceivedAndReturned, TotalsuccessCount+TotalserverReturnedFail)
	}
	sumTotal()
	detailTotal()

}

// 美观的汇总统计
func sumTotal() {
	width := 80

	fmt.Printf("\n%s\n", strings.Repeat("🌈", width/2))
	fmt.Printf("🎯 测试结果汇总统计\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 基础统计
	totalRequests := TotalSentRequests
	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(TotalsuccessCount) / float64(totalRequests) * 100
	}

	fmt.Printf("   📊 总体概况\n")
	fmt.Printf("   ├─ 📤 总发送请求数: %d\n", TotalSentRequests)
	fmt.Printf("   ├─ 📥 服务器接收并返回数: %d\n", TotalServerReceivedAndReturned)
	fmt.Printf("   ├─ 🎯 请求成功率: %.2f%%\n", successRate)
	fmt.Printf("   └─ ⚡ 总请求/响应比: %.2f%%\n",
		float64(TotalServerReceivedAndReturned)/float64(TotalSentRequests)*100)

	fmt.Printf("   \n   ✅ 成功详情\n")
	fmt.Printf("   ├─ ✅ 正确数（符合预期）: %d\n", TotalsuccessCount)
	fmt.Printf("   └─ 📈 成功率: %.2f%%\n", successRate)

	fmt.Printf("   \n   ❌ 失败分类\n")
	fmt.Printf("   ├─ 🔴 服务器返回失败数: %d (%.2f%%)\n",
		TotalserverReturnedFail,
		float64(TotalserverReturnedFail)/float64(totalRequests)*100)
	fmt.Printf("   └─ 🔴 发送失败数（无响应）: %d (%.2f%%)\n",
		TotalSendFailNoResponse,
		float64(TotalSendFailNoResponse)/float64(totalRequests)*100)

	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 健康度评估
	fmt.Printf("   🏥 系统健康度评估\n")
	if successRate >= 95 {
		fmt.Printf("   ├─ 💚 优秀 (成功率 ≥ 95%%)\n")
	} else if successRate >= 80 {
		fmt.Printf("   ├─ 💛 良好 (成功率 ≥ 80%%)\n")
	} else {
		fmt.Printf("   ├─ 💔 需改进 (成功率 < 80%%)\n")
	}

	if TotalSendFailNoResponse == 0 {
		fmt.Printf("   ├─ 🌐 网络连接: 稳定\n")
	} else {
		fmt.Printf("   ├─ 🌐 网络连接: 有%d个请求未到达服务器\n", TotalSendFailNoResponse)
	}

	fmt.Printf("   └─ 🎯 建议: %s\n", getRecommendation(successRate))

	fmt.Printf("%s\n", strings.Repeat("🌈", width/2))
}

// 详细的错误分类统计
func detailTotal() {
	if len(TotalErrTestResults) == 0 && TotalSendFailNoResponse == 0 {
		fmt.Printf("\n🎉 完美！所有请求都成功完成，没有错误！\n")
		return
	}

	width := 80
	totalErrors := len(TotalErrTestResults) + TotalSendFailNoResponse

	fmt.Printf("\n%s\n", strings.Repeat("🔍", width/2))
	fmt.Printf("📋 详细错误分析\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 错误分类统计
	errorStats := classifyErrors(TotalErrTestResults)

	fmt.Printf("   📊 错误分类统计\n")
	fmt.Printf("   ├─ 🔴 总错误数: %d\n", totalErrors)
	fmt.Printf("   ├─ 🔵 服务器返回错误: %d\n", len(TotalErrTestResults))
	fmt.Printf("   ├─ 🔴 网络发送失败: %d\n", TotalSendFailNoResponse)
	fmt.Printf("   └─ 📈 错误率: %.2f%%\n",
		float64(totalErrors)/float64(TotalSentRequests)*100)

	// 按HTTP状态码分类
	if len(errorStats.httpErrors) > 0 {
		fmt.Printf("   \n   🌐 HTTP状态码错误分布\n")
		for code, count := range errorStats.httpErrors {
			fmt.Printf("   ├─ %s: %d次\n", getHTTPStatusText(code), count)
		}
	}

	// 按业务错误码分类
	if len(errorStats.bizErrors) > 0 {
		fmt.Printf("   \n   💼 业务错误码分布\n")
		for code, count := range errorStats.bizErrors {
			fmt.Printf("   ├─ 业务码 %d: %d次\n", code, count)
		}
	}

	// 按错误消息分类
	if len(errorStats.messageErrors) > 0 {
		fmt.Printf("   \n   💬 错误消息分类\n")
		for msg, count := range errorStats.messageErrors {
			truncatedMsg := msg
			if len(truncatedMsg) > 50 {
				truncatedMsg = truncatedMsg[:47] + "..."
			}
			fmt.Printf("   ├─ \"%s\": %d次\n", truncatedMsg, count)
		}
	}

	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 显示前10个错误详情（避免输出过多）
	if len(TotalErrTestResults) > 0 {
		maxDisplay := 10
		if len(TotalErrTestResults) < maxDisplay {
			maxDisplay = len(TotalErrTestResults)
		}

		fmt.Printf("   🔍 前%d个错误详情\n", maxDisplay)
		for i := 0; i < maxDisplay; i++ {
			tr := TotalErrTestResults[i]
			fmt.Printf("   %s\n", strings.Repeat("─", 60))
			fmt.Printf("   │ 请求ID: #%d\n", tr.RequestID)
			fmt.Printf("   │ 用户: %s\n", tr.User)
			fmt.Printf("   │ 状态: HTTP %d (期望 %d)\n", tr.ActualHTTP, tr.ExpectedHTTP)
			fmt.Printf("   │ 业务码: %d (期望 %d)\n", tr.ActualBiz, tr.ExpectedBiz)
			fmt.Printf("   │ 耗时: %v\n", tr.Duration.Round(time.Millisecond))
			fmt.Printf("   │ 消息: %s\n", truncateMessage(tr.Message, 50))
		}

		if len(TotalErrTestResults) > maxDisplay {
			fmt.Printf("   │ ... 还有%d个错误未显示\n", len(TotalErrTestResults)-maxDisplay)
		}
		fmt.Printf("   %s\n", strings.Repeat("─", 60))
	}

	// 网络错误单独显示
	if TotalSendFailNoResponse > 0 {
		fmt.Printf("   \n   🌐 网络发送失败\n")
		fmt.Printf("   ├─ 🔴 无响应请求: %d个\n", TotalSendFailNoResponse)
		fmt.Printf("   └─ 💡 可能原因: 网络超时、连接拒绝、服务器宕机\n")
	}

	fmt.Printf("%s\n", strings.Repeat("🔍", width/2))
}

// 错误分类结构体
type ErrorStats struct {
	httpErrors    map[int]int    // HTTP状态码错误
	bizErrors     map[int]int    // 业务错误码
	messageErrors map[string]int // 错误消息
}

// 错误分类函数
func classifyErrors(results []TestResult) ErrorStats {
	stats := ErrorStats{
		httpErrors:    make(map[int]int),
		bizErrors:     make(map[int]int),
		messageErrors: make(map[string]int),
	}

	for _, result := range results {
		// 按HTTP状态码分类
		if result.ActualHTTP != result.ExpectedHTTP {
			stats.httpErrors[result.ActualHTTP]++
		}

		// 按业务错误码分类
		if result.ActualBiz != result.ExpectedBiz {
			stats.bizErrors[result.ActualBiz]++
		}

		// 按错误消息分类
		if result.Message != "" && result.Message != "无响应" {
			stats.messageErrors[result.Message]++
		}
	}

	return stats
}

// 辅助函数
func getHTTPStatusText(code int) string {
	statusTexts := map[int]string{
		400: "400 Bad Request",
		401: "401 Unauthorized",
		403: "403 Forbidden",
		404: "404 Not Found",
		429: "429 Too Many Requests",
		500: "500 Internal Server Error",
		502: "502 Bad Gateway",
		503: "503 Service Unavailable",
	}

	if text, exists := statusTexts[code]; exists {
		return text
	}
	return fmt.Sprintf("%d Unknown", code)
}

func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}

func getRecommendation(successRate float64) string {
	if successRate >= 95 {
		return "系统表现优秀，继续保持当前配置"
	} else if successRate >= 80 {
		return "系统表现良好，建议检查个别失败请求的原因"
	} else if successRate >= 60 {
		return "系统表现一般，建议优化网络连接或检查服务器状态"
	} else {
		return "系统表现较差，需要详细排查网络、服务器和代码逻辑"
	}
}

// 新的辅助函数，使用独立的Token上下文
func testFuncWithContext(t *testing.T, userID int, username, password string, userCtx *TestContext) (bool, *APIResponse, int, int) {
	start := time.Now()

	// 使用传入的独立Token上下文，不再重新登录
	if userCtx == nil || userCtx.AccessToken == "" {
		t.Logf("用户%d Token为空", userID)
		return false, nil, http.StatusCreated, RespCodeSuccess
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

	// 使用独立的Token发送请求
	createResp, err := sendTokenRequest(userCtx, http.MethodPost, UsersAPIPath, bytes.NewReader(jsonData))
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
}

// 分批并发测试
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		runConcurrentTest(t, testName, startUser, endUser)

		// 批次间休息，释放资源
		if batch < totalBatches-1 {
			fmt.Printf("⏸️  批次间休息 2秒...\n")
			time.Sleep(2 * time.Second)
			if transport, ok := httpClient.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
			runtime.GC()
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// ==================== 并发测试框架 ====================
func runConcurrentTest(t *testing.T, testName string, startUser, endUser int) {
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
	fmt.Printf("📊 并发用户: %d-%d, 每用户请求: %d, 总请求: %d\n",
		startUser, endUser-1, RequestsPerUser, (endUser-startUser)*RequestsPerUser)
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()
	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	totalDuration := time.Duration(0)

	// 统计信息显示协程
	statsTicker := time.NewTicker(500 * time.Millisecond)
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
				fmt.Printf("\033[4;1H\033[K")
				fmt.Printf("   ✅ 成功请求: %d\n", currentSuccess)
				fmt.Printf("\033[5;1H\033[K")
				fmt.Printf("   ❌ 失败请求: %d\n", currentFail)
				fmt.Printf("\033[6;1H\033[K")
				fmt.Printf("   📈 成功率: %.1f%%\n", float64(currentSuccess)/float64(totalRequests)*100)
				fmt.Printf("\033[7;1H\033[K")
				fmt.Printf("   ⏱️  当前耗时: %v\n", currentDuration.Round(time.Millisecond))
				fmt.Printf("\033[8;1H\033[K")
				fmt.Printf("   🚀 实时QPS: %.1f\n", float64(totalRequests)/currentDuration.Seconds())
				if totalDuration > 0 && successCount > 0 {
					fmt.Printf("\033[9;1H\033[K")
					fmt.Printf("   ⚡ 平均耗时: %v\n", totalDuration/time.Duration(successCount))
				}
				fmt.Printf("\033[10;1H\033[K")
				fmt.Printf("%s", strings.Repeat("─", width))
			}
		}
	}()

	// 启动并发测试 - 关键修改：每个用户单独管理Token
	for i := startUser; i < endUser; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			// 每个用户单独登录，获取独立的Token
			userCtx, _, err := login(TestUsername, ValidPassword)
			if err != nil {
				//	log.Errorf("用户%d登录失败: %v", userID, err)
				// 记录这个用户的所有请求都失败
				mu.Lock()
				TotalSentRequests += RequestsPerUser
				TotalSendFailNoResponse += RequestsPerUser
				failCount += RequestsPerUser
				mu.Unlock()
				return
			}

			username := TestUsername
			password := ValidPassword

			for j := 0; j < RequestsPerUser; j++ {
				requestID := userID*RequestsPerUser + j + 1

				start := time.Now()
				// 使用独立的Token上下文调用测试函数
				success, resp, expectedHTTP, expectedBiz := testFuncWithContext(t, userID, username, password, userCtx)
				duration := time.Since(start)

				mu.Lock()
				TotalSentRequests++

				if success {
					successCount++
					TotalsuccessCount++
					totalDuration += duration
					TotalServerReceivedAndReturned++
				} else {
					if resp != nil {
						TotalserverReturnedFail++
						TotalServerReceivedAndReturned++
					} else {
						TotalSendFailNoResponse++
					}
					failCount++
				}

				if resp != nil {
					if !success {
						TotalErrTestResults = append(TotalErrTestResults, TestResult{
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
					}
				} else {
					TotalErrTestResults = append(TotalErrTestResults, TestResult{
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
	statsTicker.Stop()

	// 输出最终结果
	duration := time.Since(startTime)
	totalRequests := (endUser - startUser) * RequestsPerUser

	fmt.Printf("\033[11;1H\033[K")
	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("📊 批次测试完成!\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))
	fmt.Printf("   ✅ 总成功数: %d/%d (%.1f%%)\n", successCount, totalRequests, float64(successCount)/float64(totalRequests)*100)
	fmt.Printf("   ❌ 总失败数: %d/%d (%.1f%%)\n", failCount, totalRequests, float64(failCount)/float64(totalRequests)*100)
	fmt.Printf("   ⏱️  总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("   🚀 平均QPS: %.1f\n", float64(totalRequests)/duration.Seconds())
	if successCount > 0 {
		fmt.Printf("   ⚡ 平均响应时间: %v\n", totalDuration/time.Duration(successCount))
	}
	fmt.Printf("%s\n", strings.Repeat("═", width))

	// 强制垃圾回收
	runtime.GC()
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

	// if resp.StatusCode == http.StatusOK {
	// 	log.Infof("登录成功，获取到Token: access_token长度=%d", len(accessToken))
	// }

	return &TestContext{
		Username:     username,
		Userid:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, &apiResp, nil
}

func sendTokenRequest(ctx *TestContext, method, path string, body io.Reader) (*APIResponse, error) {
	if ctx == nil || ctx.AccessToken == "" {
		return nil, fmt.Errorf("Token为空或上下文为空")
	}

	// 添加Token验证
	if len(ctx.AccessToken) < 10 {
		log.Warnf("Token长度异常: %d", len(ctx.AccessToken))
	}

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

	return &apiResp, nil
}

func generateValidUserName(userID int) string {
	timestamp := time.Now().UnixNano() % 10000
	return fmt.Sprintf("user_%d_%d_%d", userID, timestamp, rand.IntN(1000000))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== 主测试函数 ====================
func TestMain(m *testing.M) {
	fmt.Println("初始化测试环境...")
	checkResourceLimits()
	setHigherFileLimit()
	code := m.Run()
	os.Exit(code)
}

func checkResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		fmt.Printf("当前文件描述符限制: Soft=%d, Hard=%d\n", rLimit.Cur, rLimit.Max)
		if rLimit.Cur < 10000 {
			fmt.Printf("⚠️  文件描述符限制较低，建议使用: ulimit -n 10000\n")
		}
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

// 创建优化的HTTP客户端
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:          50,
			MaxIdleConnsPerHost:   50,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			DisableKeepAlives:     false,
		},
	}
}
