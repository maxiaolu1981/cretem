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
	"strconv"
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
	RequestTimeout = 10 * time.Second // 修改：增加超时时间

	LoginAPIPath = "/login"
	UsersAPIPath = "/v1/users"

	TestUsername    = "admin"
	ValidPassword   = "Admin@2021"
	InvalidPassword = "Admin@2022"

	RespCodeSuccess    = 100001
	RespCodeValidation = 100400
	RespCodeConflict   = 100409

	ConcurrentUsers = 100                  // 修改：降低并发数，逐步增加
	RequestsPerUser = 10                   // 修改：减少每个用户的请求数
	RequestInterval = 0 * time.Millisecond // 修改：增加请求间隔
	BatchSize       = 100                  // 新增：批次大小
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
	// 修改：优化HTTP客户端连接池配置
	httpClient  = createHTTPClient()
	mu          sync.Mutex
	testResults []TestResult
)

// 新增：创建优化的HTTP客户端
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   50,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			// 新增：禁用长连接（对于测试可能有用）
			DisableKeepAlives: false, // 保持开启，但可以尝试设为true测试
		},
	}
}

// ==================== 主测试函数 ====================
func TestMain(m *testing.M) {
	fmt.Println("初始化测试环境...")

	// 新增：检查系统资源限制
	checkResourceLimits()

	// 新增：设置更高的文件描述符限制（如果可能）
	setHigherFileLimit()

	// 运行测试
	code := m.Run()
	os.Exit(code)
}

// 新增：检查资源限制
func checkResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		fmt.Printf("当前文件描述符限制: Soft=%d, Hard=%d\n", rLimit.Cur, rLimit.Max)
		if rLimit.Cur < 10000 {
			fmt.Printf("⚠️  文件描述符限制较低，建议使用: ulimit -n 10000\n")
		}
	}
}

// 新增：尝试设置更高的文件描述符限制
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

func TestCase_CreateUserSuccess_Concurrent(t *testing.T) {
	// 修改：使用分批测试
	runBatchConcurrentTest(t, "创建用户成功并发测试", func(t *testing.T, userID int, username, password string) (bool, *APIResponse, int, int) {
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
		// log.Errorf("调试: 用户=%v  期望HTTP=%v 期望业务码=%v 实际HTTP=%v  实际业务码=%v",
		// 	userID,
		// 	http.StatusCreated,
		// 	RespCodeSuccess,
		// 	createResp.HTTPStatus,
		// 	createResp.Code)
		duration := time.Since(start)
		if !success {
			t.Logf("用户请求 %d 创建失败: HTTP=%d, Code=%d, Message=%s, 耗时: %v",
				userID, createResp.HTTPStatus, createResp.Code, createResp.Message, duration)
		} else {
			t.Logf("用户请求 %d 创建成功, 耗时: %v", userID, duration)
		}

		return success, createResp, http.StatusCreated, RespCodeSuccess
	})

	totalErr := []TestResult{}
	for _, tr := range testResults {
		if !strings.Contains(strconv.Itoa(tr.ExpectedHTTP), "201") {
			totalErr = append(totalErr, tr)
		}
	}
	if len(totalErr) > 0 {
		fmt.Printf("错误明细\n")
		for _, tr := range totalErr {
			fmt.Println(tr)
		}
		fmt.Printf("%s\n", strings.Repeat("═", 80))
		fmt.Printf("   ✅  错误总计: %d\n", len(totalErr))
	}
}

// 新增：分批并发测试
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		runConcurrentTest(t, testName, startUser, endUser, testFunc)

		// 批次间休息，释放资源
		if batch < totalBatches-1 {
			fmt.Printf("⏸️  批次间休息 2秒...\n")
			time.Sleep(2 * time.Second)
			// 强制关闭空闲连接
			if transport, ok := httpClient.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
			// 新增：强制垃圾回收
			runtime.GC()
			time.Sleep(500 * time.Millisecond)
		}
	}
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
	//log.Errorf("响应格式错误%+v", apiResp)
	tokenData, ok := apiResp.Data.(map[string]interface{})
	if !ok {

		return nil, &apiResp, fmt.Errorf("响应格式错误")
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	userID, _ := tokenData["user_id"].(string)

	if resp.StatusCode == http.StatusOK {
		log.Errorf("登录成功，获取到Token: access_token长度=%d, refresh_token长度=%d",
			len(accessToken), len(refreshToken))
		log.Errorf("AccessToken: %s", accessToken) // 注意：生产环境不要日志真实Token
	}

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

// ==================== 并发测试框架 ====================
func runConcurrentTest(t *testing.T, testName string, startUser, endUser int, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {

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
	successCount := 0 // 对应“正确数”（已有
	failCount := 0    //failCount := 0
	totalDuration := time.Duration(0)

	// 新增：4个指标的统计变量
	totalSentRequests := 0         // 1. 总发送请求数（尝试发送的总数）
	serverReceivedAndReturned := 0 // 2. 服务器接收并返回的请求数（有响应）
	serverReturnedFail := 0        // 3. 服务器返回失败数（有响应但不符合预期）
	sendFailNoResponse := 0        // 4. 发送失败数（无响应，请求没到服务器）

	//progress := make(chan string, 100)
	//done := make(chan bool)

	// 新增：内存监控协程
	//stopMonitor := make(chan bool)
	//go monitorMemoryUsage(stopMonitor)

	// 进度显示协程
	// go func() {
	// 	line := 6
	// 	for msg := range progress {
	// 		fmt.Printf("\033[%d;1H", line)
	// 		fmt.Printf("\033[K")
	// 		fmt.Printf("   %s", msg)
	// 		line++
	// 		if line > 10 {
	// 			line = 6
	// 		}
	// 	}
	// 	done <- true
	// }()

	// 统计信息显示协程
	statsTicker := time.NewTicker(500 * time.Millisecond) // 修改：降低刷新频率
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

	// 启动并发测试
	for i := startUser; i < endUser; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			username := TestUsername
			password := ValidPassword

			for j := 0; j < RequestsPerUser; j++ {
				requestID := userID*RequestsPerUser + j + 1
				//	progress <- fmt.Sprintf("🟡 [用户%d] 请求 %d 开始...", userID, j+1)

				start := time.Now()
				success, resp, expectedHTTP, expectedBiz := testFunc(t, userID, username, password)
				duration := time.Since(start)

				mu.Lock()
				// 1. 先更新“总发送请求数”（每执行一次请求，就计数一次）
				totalSentRequests++

				if success {
					successCount++
					totalDuration += duration
					// <- fmt.Sprintf("🟢 [用户%d] 请求 %d 成功 (耗时: %v)", userID, j+1, duration)
					// 正确数属于“服务器接收并返回”，所以该请求需计入 serverReceivedAndReturned
					serverReceivedAndReturned++
				} else {
					// 失败情况：区分“服务器返回失败”和“发送失败无响应”
					if resp != nil {
						// 服务器接收并返回，但结果不符合预期（如HTTP 400、业务码错误）
						serverReturnedFail++
						serverReceivedAndReturned++ // 有响应，计入服务器接收数
						//	progress <- fmt.Sprintf("🔴 [用户%d] 请求 %d 失败（服务器返回）: %s (耗时: %v)",
						//		userID, j+1, resp.Message, duration)
					} else {
						// 发送失败：请求没到服务器（如网络错误、连接超时）
						sendFailNoResponse++
						//	progress <- fmt.Sprintf("🔴 [用户%d] 请求 %d 失败（发送失败）: 无响应 (耗时: %v)",
						//		userID, j+1, duration)
					}
					failCount++
					//	progress <- fmt.Sprintf("🔴 [用户%d] 请求 %d 失败 (耗时: %v)", userID, j+1, duration)
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
	//close(progress)
	//<-done
	statsTicker.Stop()
	//stopMonitor <- true

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

	// 在打印新增指标前，添加校验（放在fmt.Printf("%s\n", strings.Repeat("─", width))之前）
	// 校验逻辑：总发送数 = 正确数 + 服务器返回失败数 + 发送失败数
	if totalSentRequests != successCount+serverReturnedFail+sendFailNoResponse {
		fmt.Printf("   ⚠️  统计校验警告：数据不匹配！总发送数=%d，正确数+返回失败数+发送失败数=%d\n",
			totalSentRequests, successCount+serverReturnedFail+sendFailNoResponse)
	}
	// 校验逻辑：服务器接收并返回数 = 正确数 + 服务器返回失败数
	if serverReceivedAndReturned != successCount+serverReturnedFail {
		fmt.Printf("   ⚠️  统计校验警告：服务器接收数不匹配！接收数=%d，正确数+返回失败数=%d\n",
			serverReceivedAndReturned, successCount+serverReturnedFail)
	}

	// 原有最终统计代码之后，新增以下打印
	fmt.Printf("%s\n", strings.Repeat("─", width))
	fmt.Printf("汇总统计\n")
	// 新增：4个核心指标打印
	fmt.Printf("   📤 总发送请求数: %d\n", totalSentRequests)
	fmt.Printf("   📥 服务器接收并返回数: %d\n", serverReceivedAndReturned)
	fmt.Printf("   ✅ 正确数（符合预期）: %d\n", successCount)
	fmt.Printf("   ❌ 服务器返回失败数: %d\n", serverReturnedFail)
	fmt.Printf("   ❌ 发送失败数（无响应）: %d\n", sendFailNoResponse)
	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 强制垃圾回收
	runtime.GC()
}

// 新增：内存监控函数
func monitorMemoryUsage(stop chan bool) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("\033[22;1H\033[K")
			fmt.Printf("💾 内存使用: Alloc=%.1fMB, Goroutines=%d, GC次数=%d",
				float64(m.Alloc)/1024/1024,
				runtime.NumGoroutine(),
				m.NumGC)
		case <-stop:
			return
		}
	}
}
