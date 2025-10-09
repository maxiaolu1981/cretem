/*
1. 主测试套件 (1个)
TestAllSingleUserGetTests - 运行所有单用户查询测试的完整套件

2. 核心功能测试 (4个)
🔍 边界情况测试
TestSingleUserGetEdgeCases - 测试各种边界和异常情况

空用户ID

超长用户ID（1000字符）

特殊字符用户ID

SQL注入尝试

纯数字用户ID

🔐 认证测试
TestSingleUserGetAuthentication - 测试认证和授权机制

无Token请求

无效Token

过期Token

格式错误Token

⚡ 并发压力测试
TestSingleUserGetConcurrent - 大并发压力测试

50% 热点用户请求

20% 无效用户请求

10% 无权限用户请求

20% 随机正常用户请求

支持批量并发（1000用户×200请求）

🚨 缓存相关测试 (3个)
缓存击穿测试
TestSingleUserGetCachePenetration - 专门测试缓存击穿防护

使用不存在的用户ID进行高并发请求

监测数据库查询频率

检测缓存击穿风险

热点Key测试
TestSingleUserGetHotKey - 测试热点用户处理能力

对同一个热点用户进行1000次并发请求

监测响应时间和吞吐量

评估热点数据处理性能

冷启动测试
TestSingleUserGetColdStart - 测试缓存冷启动性能

第一次请求（冷启动）耗时

第二次请求（预热后）耗时

计算缓存性能提升比例

🎯 测试用例统计
测试类型	用例数量	主要作用
边界测试	1个用例（5种场景）	验证异常输入处理
认证测试	1个用例（4种场景）	验证安全机制
并发测试	1个用例（4种请求类型）	压力性能和稳定性
缓存测试	3个用例	缓存相关特殊场景
总计	6个主要测试函数	覆盖14+种测试场景
🚀 测试运行方式
bash
# 运行完整测试套件
go test -v -run TestAllSingleUserGetTests -timeout=30m

# 单独运行缓存击穿测试
go test -v -run TestSingleUserGetCachePenetration -timeout=10m

# 单独运行并发压力测试
go test -v -run TestSingleUserGetConcurrent -timeout=15m

# 运行所有缓存相关测试
go test -v -run "TestSingleUserGetCache|TestSingleUserGetHot|TestSingleUserGetCold" -timeout=20m

压力测试配置（高并发）
ConcurrentUsers       = 1000    // 更多并发用户
RequestsPerUser       = 100     // 每个用户较少请求
RequestInterval       = 10 * time.Millisecond  // 更短间隔

稳定性测试配置（长时间运行）
ConcurrentUsers       = 100     // 适中并发
RequestsPerUser       = 1000    // 每个用户更多请求
RequestInterval       = 100 * time.Millisecond // 正常间隔

缓存击穿测试配置
HotUserRequestPercent = 0       // 不使用热点用户
InvalidRequestPercent = 100     // 100%无效请求
ConcurrentUsers       = 50      // 适中并发

// 场景1：正常压力测试
ConcurrentUsers = 100       // 100个并发用户
RequestsPerUser = 200       // 每个用户200次请求
// 总请求: 20,000次

// 场景2：缓存击穿测试
ConcurrentUsers = 50        // 50个并发用户
RequestsPerUser = 1000      // 每个用户1000次请求
InvalidRequestPercent = 100 // 全部请求不存在的用户
// 总请求: 50,000次，全部触发缓存未命中

*/

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ==================== 配置常量 ====================
const (
	ServerBaseURL  = "http://192.168.10.8:8088"
	RequestTimeout = 30 * time.Second

	LoginAPIPath   = "/login"
	SingleUserPath = "/v1/users/%s"

	TestUsername  = "admin"
	ValidPassword = "Admin@2021"

	RespCodeSuccess    = 100001
	RespCodeNotFound   = 100206 // 根据实际系统调整为110001
	RespCodeForbidden  = 110009 //无权访问
	RespCodeValidation = 100400

	ConcurrentUsers       = 10
	RequestsPerUser       = 10
	RequestInterval       = 1 * time.Millisecond
	BatchSize             = 1
	HotUserRequestPercent = 50 // 热点用户请求百分比
	InvalidRequestPercent = 10 //无效请求百分比

	P50Threshold   = 50 * time.Millisecond
	P90Threshold   = 100 * time.Millisecond
	P99Threshold   = 200 * time.Millisecond
	ErrorRateLimit = 0.01

	// 缓存击穿测试相关常量
	CachePenetrationTestUsers  = 1000                      // 缓存击穿测试并发用户数
	CachePenetrationRequests   = 100                       // 每个用户请求次数
	CachePenetrationUserID     = "mxl_nonexistent-user-11" // 用于缓存击穿测试的用户ID
	CachePenetrationBatchDelay = 300 * time.Millisecond    // 批次间延迟
)

// 在全局变量部分添加预定义的用户列表
var (
	// 预定义的有效用户列表
	predefinedValidUsers = []string{
		"admin",
	}

	// 预定义的热点用户
	predefinedHotUser = "admin"

	// 预定义的无权限用户
	predefinedUnauthorizedUser = "user_85_201789_127851"

	// 预定义的无效用户列表
	predefinedInvalidUsers = []string{
		"nonexistent-user-001",
		"invalid-user-123",
		"deleted-user-456",
		"test-invalid-789",
		"fake-user-000",
		"user-not-found-999",
		"unknown-user-888",
		"ghost-user-777",
		"deleted-account-666",
		"removed-user-555",
	}
)

// ==================== 数据结构 ====================
type APIResponse struct {
	HTTPStatus int         `json:""`
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

type PerformanceStats struct {
	TotalRequests       int
	SuccessCount        int
	ExpectedFailCount   int // 预期的失败（如404）
	UnexpectedFailCount int // 意外的失败（如500）
	TotalDuration       time.Duration
	Durations           []time.Duration
	StatusCount         map[int]int
	BusinessCodeCount   map[int]int
}

// ==================== 全局变量 ====================
var (
	httpClient       = createHTTPClient()
	mu               sync.Mutex
	validUserIDs     []string
	hotUserID        string
	invalidUserIDs   []string
	unauthorizedUser string

	cachePenetrationCounter int
	cachePenetrationMutex   sync.Mutex
	TotalStats              = &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}
	TotalErrTestResults = []TestResult{}
)

type TestResult struct {
	Username     string
	ExpectedHTTP int
	ExpectedBiz  int
	ActualHTTP   int
	ActualBiz    int
	Message      string
}

// ==================== 初始化函数 ====================
func initTestData() {
	// 使用预定义的用户列表
	// 过滤掉有问题的用户
	filteredUsers := []string{}
	for _, user := range predefinedValidUsers {
		// 移除已知有问题的用户
		if !isProblematicUser(user) {
			filteredUsers = append(filteredUsers, user)
		}
	}

	validUserIDs = predefinedValidUsers
	hotUserID = predefinedHotUser
	unauthorizedUser = predefinedUnauthorizedUser
	invalidUserIDs = predefinedInvalidUsers

	fmt.Printf("✅ 初始化测试数据完成:\n")
	fmt.Printf("   有效用户数量: %d\n", len(validUserIDs))
	fmt.Printf("   热点用户: %s\n", hotUserID)
	fmt.Printf("   无权限用户: %s\n", unauthorizedUser)
	fmt.Printf("   无效用户数量: %d\n", len(invalidUserIDs))

	if len(validUserIDs) > 10 {
		fmt.Printf("   示例用户: %v\n", validUserIDs[:10])
	} else {
		fmt.Printf("   所有用户: %v\n", validUserIDs)
	}
}

func isProblematicUser(username string) bool {
	problemUsers := map[string]bool{
		"test_user_123":      true, // 返回500
		"user_0_1079_314004": true, // 返回422
		// 添加其他有问题的用户
	}
	return problemUsers[username]
}

func init() {
	fmt.Println("初始化单用户查询接口测试环境...")
	initTestData()
	checkResourceLimits()
	setHigherFileLimit()
}

// ==================== HTTP客户端函数 ====================
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 90 * time.Second,
			}).DialContext,
			MaxIdleConns:          1000,
			MaxIdleConnsPerHost:   1000,
			MaxConnsPerHost:       1000,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// ==================== 登录函数 ====================
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

	// 首先尝试解析为标准API响应格式
	var apiResp APIResponse
	apiResp.HTTPStatus = resp.StatusCode
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		// 如果解析失败，说明不是标准格式，尝试直接解析为token数据
		return parseDirectLoginResponse(resp, respBody, username)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &apiResp, fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	// 检查是否是标准格式（包含code、message、data字段）
	if apiResp.Data != nil {
		// 标准格式：从Data字段提取token
		tokenData, ok := apiResp.Data.(map[string]interface{})
		if !ok {
			return nil, &apiResp, fmt.Errorf("响应Data字段格式错误")
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
	} else {
		// 非标准格式，直接解析整个响应体
		return parseDirectLoginResponse(resp, respBody, username)
	}
}

// parseDirectLoginResponse 解析直接返回的登录响应（非标准格式）
func parseDirectLoginResponse(resp *http.Response, respBody []byte, username string) (*TestContext, *APIResponse, error) {
	// 直接解析为token数据
	var tokenData map[string]interface{}
	if err := json.Unmarshal(respBody, &tokenData); err != nil {

		return nil, nil, fmt.Errorf("响应格式错误: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 构建错误响应
		apiResp := &APIResponse{
			HTTPStatus: resp.StatusCode,
			Code:       -1,
			Message:    "登录失败",
			Data:       tokenData,
		}

		return nil, apiResp, fmt.Errorf("登录失败: HTTP %d", resp.StatusCode)
	}

	accessToken, _ := tokenData["access_token"].(string)
	refreshToken, _ := tokenData["refresh_token"].(string)
	userID, _ := tokenData["user_id"].(string)

	if accessToken == "" {
		return nil, nil, fmt.Errorf("登录失败: 未获取到access_token")
	}

	// 构建成功的API响应
	apiResp := &APIResponse{
		HTTPStatus: resp.StatusCode,
		Code:       200,
		Message:    "登录成功",
		Data:       tokenData,
	}

	return &TestContext{
		Username:     username,
		Userid:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, apiResp, nil
}

// ==================== 请求发送函数 ====================
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
	apiResp.HTTPStatus = resp.StatusCode
	if err := json.Unmarshal(respBody, &apiResp); err != nil {

		return nil, err
	}

	return &apiResp, nil
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== 并发测试框架 ====================
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	// 每轮测试前重置明细
	mu.Lock()
	TotalErrTestResults = []TestResult{}
	mu.Unlock()

	// 先登录获取token
	ctx, _, err := login(TestUsername, ValidPassword)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	fmt.Println("totalBatches", totalBatches)
	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		runConcurrentTest(t, startUser, endUser, ctx, testFunc)

		// 批次间休息
		if batch < totalBatches-1 {
			runtime.GC()
		}
	}

	mu.Lock()
	fmt.Printf("[DEBUG] printPerformanceReport 前明细数量: %d\n", len(TotalErrTestResults))
	mu.Unlock()
	// 输出性能报告
	printPerformanceReport(testName)
	mu.Lock()
	fmt.Printf("[DEBUG] printPerformanceReport 后明细数量: %d\n", len(TotalErrTestResults))
	mu.Unlock()
}

func runConcurrentTest(t *testing.T,
	startUser, endUser int, ctx *TestContext,
	testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) *PerformanceStats {
	var wg sync.WaitGroup
	// 启动并发测试
	for i := startUser; i < endUser; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			for j := 0; j < RequestsPerUser; j++ {
				success, isExpectedFailure, _, _, _ := testFunc(t, userID, ctx)
				mu.Lock()
				TotalStats.TotalRequests++
				if success {
					TotalStats.SuccessCount++
					if isExpectedFailure {
						// 这是预期的失败（如正确的404响应），也算在预期失败中
						TotalStats.ExpectedFailCount++
					}
				} else {
					if isExpectedFailure {
						// 这是预期失败的情况，但响应不符合预期
						TotalStats.UnexpectedFailCount++
					} else {
						//mxl
						// 这是真正的意外失败
						TotalStats.UnexpectedFailCount++
					}
				}
				mu.Unlock()
				time.Sleep(RequestInterval)
			}
		}(i)
	}
	wg.Wait()
	runtime.GC()
	return TotalStats
}

func testSingleUserGetRequest(t *testing.T, userID int, ctx *TestContext) (bool, bool, *APIResponse, int, int) {
	var targetUserID string
	var expectedHTTP int
	var expectedBiz int
	isExpectedFailure := false

	// 随机决定请求类型
	randNum := rand.IntN(100)
	//randNum := 90
	switch {
	case randNum < HotUserRequestPercent:
		// 每次都重新登录获取 token
		loginCtx, _, loginErr := login(TestUsername, ValidPassword)
		if loginErr != nil || loginCtx == nil {
			return false, false, nil, http.StatusUnauthorized, RespCodeNotFound
		}
		targetUserID = hotUserID
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
		resp, err := sendTokenRequest(loginCtx, http.MethodGet, fmt.Sprintf(SingleUserPath, targetUserID), nil)
		return checkGetResp(resp, err, loginCtx, expectedHTTP, expectedBiz, isExpectedFailure)
	case randNum < HotUserRequestPercent+InvalidRequestPercent:
		loginCtx, _, loginErr := login(TestUsername, ValidPassword)
		if loginErr != nil || loginCtx == nil {
			return false, true, nil, http.StatusUnauthorized, RespCodeNotFound
		}
		targetUserID = invalidUserIDs[rand.IntN(len(invalidUserIDs))]
		expectedHTTP = http.StatusUnauthorized
		expectedBiz = RespCodeNotFound
		isExpectedFailure = true
		resp, err := sendTokenRequest(loginCtx, http.MethodGet, fmt.Sprintf(SingleUserPath, targetUserID), nil)
		return checkGetResp(resp, err, loginCtx, expectedHTTP, expectedBiz, isExpectedFailure)
	case randNum < HotUserRequestPercent+InvalidRequestPercent+10:
		// 先用无权限用户登录获取 token
		unauthorizedCtx, _, loginErr := login(unauthorizedUser, ValidPassword)
		if loginErr != nil || unauthorizedCtx == nil {
			// 登录失败，直接返回 401
			return false, true, nil, http.StatusUnauthorized, RespCodeNotFound
		}
		targetUserID = unauthorizedUser
		expectedHTTP = http.StatusForbidden
		expectedBiz = RespCodeForbidden
		isExpectedFailure = true
		resp, err := sendTokenRequest(unauthorizedCtx, http.MethodGet, fmt.Sprintf(SingleUserPath, targetUserID), nil)
		return checkGetResp(resp, err, unauthorizedCtx, expectedHTTP, expectedBiz, isExpectedFailure)
	default:
		loginCtx, _, loginErr := login(TestUsername, ValidPassword)
		if loginErr != nil || loginCtx == nil {
			return false, false, nil, http.StatusUnauthorized, RespCodeNotFound
		}
		targetUserID = validUserIDs[rand.IntN(len(validUserIDs))]
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
		resp, err := sendTokenRequest(loginCtx, http.MethodGet, fmt.Sprintf(SingleUserPath, targetUserID), nil)
		return checkGetResp(resp, err, loginCtx, expectedHTTP, expectedBiz, isExpectedFailure)
	}
}

// checkGetResp 统一校验响应
func checkGetResp(resp *APIResponse, err error, ctx *TestContext, expectedHTTP, expectedBiz int, isExpectedFailure bool) (bool, bool, *APIResponse, int, int) {
	if err != nil {
		mu.Lock()
		TotalErrTestResults = append(TotalErrTestResults,
			TestResult{
				Username:     ctx.Username,
				ExpectedHTTP: expectedHTTP,
				ActualHTTP:   -1,
				ExpectedBiz:  expectedBiz,
				ActualBiz:    -1,
				Message:      "请求异常: " + err.Error(),
			})
		fmt.Printf("[DEBUG] append error: %s\n", err.Error())
		mu.Unlock()
		return false, isExpectedFailure, nil, expectedHTTP, expectedBiz
	}
	success := (resp.HTTPStatus == expectedHTTP) && (resp.Code == expectedBiz)
	if !success {
		msg := ""
		if resp != nil {
			msg = fmt.Sprintf("期望HTTP:%d/业务码:%d，实际HTTP:%d/业务码:%d，message:%s，error:%s", expectedHTTP, expectedBiz, resp.HTTPStatus, resp.Code, resp.Message, resp.Error)
		} else {
			msg = "响应为空"
		}
		mu.Lock()
		TotalErrTestResults = append(TotalErrTestResults,
			TestResult{
				Username:     ctx.Username,
				ExpectedHTTP: expectedHTTP,
				ActualHTTP:   resp.HTTPStatus,
				ExpectedBiz:  expectedBiz,
				ActualBiz:    resp.Code,
				Message:      msg,
			})
		fmt.Printf("[DEBUG] append error: %s\n", msg)
		mu.Unlock()
	}
	return success, isExpectedFailure, resp, expectedHTTP, expectedBiz
}

// ==================== 性能报告函数 ====================
func printPerformanceReport(testName string) {
	width := 60
	separator := strings.Repeat("─", width)
	thickSeparator := strings.Repeat("═", width)

	fmt.Printf("\n")
	fmt.Printf("┌%s┐\n", thickSeparator)
	fmt.Printf("│%s│\n", centerText("📊 "+testName+" 性能报告", width))
	fmt.Printf("├%s┤\n", separator)

	// 基础统计 - 添加除零保护
	totalRequests := TotalStats.TotalRequests
	if totalRequests == 0 {
		totalRequests = 1 // 避免除零
	}

	fmt.Printf("│ %-25s: %8d │\n", "总请求数", TotalStats.TotalRequests)

	// 使用安全的百分比计算
	successPercent := safePercent(TotalStats.SuccessCount, totalRequests)
	expectedFailPercent := safePercent(TotalStats.ExpectedFailCount, totalRequests)
	unexpectedFailPercent := safePercent(TotalStats.UnexpectedFailCount, totalRequests)

	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "成功数", TotalStats.SuccessCount, successPercent)
	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "预期失败数", TotalStats.ExpectedFailCount, expectedFailPercent)
	fmt.Printf("│ %-25s: %8d (%.2f%%) │\n", "意外失败数", TotalStats.UnexpectedFailCount, unexpectedFailPercent)

	fmt.Printf("├%s┤\n", separator)
	fmt.Printf("├%s┤\n", separator)

	// 错误率分析 - 使用安全计算
	realErrorRate := safePercent(TotalStats.UnexpectedFailCount, totalRequests) / 100
	errorRateDisplay := fmt.Sprintf("%.4f%%", realErrorRate*100)
	if realErrorRate <= ErrorRateLimit {
		fmt.Printf("│ %-25s: %8s ✅ │\n", "真实错误率", errorRateDisplay)
	} else {
		fmt.Printf("│ %-25s: %8s ❌ │\n", "真实错误率", errorRateDisplay)
	}

	// 错误分析
	if TotalStats.UnexpectedFailCount > 0 {
		fmt.Printf("├%s┤\n", separator)
		fmt.Printf("│ %-56s │\n", "🔍 错误分析:")

		fmt.Printf("│   真正的意外错误: %d次%*s│\n",
			TotalStats.UnexpectedFailCount, 38-len(fmt.Sprintf("真正的意外错误: %d次", TotalStats.UnexpectedFailCount)), "")
		fmt.Printf("│    错误明细如下：%*s│\n", 44, "")
		maxErrs := 20
		shown := 0
		// 只输出真正的意外失败明细（ExpectedHTTP != ActualHTTP || ExpectedBiz != ActualBiz）
		for i := 0; i < len(TotalErrTestResults) && shown < maxErrs; i++ {
			tr := TotalErrTestResults[i]
			if tr.ExpectedHTTP != tr.ActualHTTP || tr.ExpectedBiz != tr.ActualBiz {
				msg := tr.Message
				if msg == "" {
					msg = fmt.Sprintf("%v", tr)
				}
				errMsg := truncateText(msg, 100)
				fmt.Printf("│    %2d. %-96s│\n", shown+1, errMsg)
				shown++
			}
		}
		if shown == 0 {
			fmt.Printf("│    无详细错误信息，请检查 checkGetResp/TotalErrTestResults 填充逻辑！%*s│\n", 44, "")
		}
		if TotalStats.UnexpectedFailCount > maxErrs {
			fmt.Printf("│    ...仅显示前%d条，更多请查日志...%*s│\n", maxErrs, 70-len(fmt.Sprintf("...仅显示前%d条，更多请查日志...", maxErrs)), "")
		}
	}
	// 其他错误状态码
	for status, count := range TotalStats.StatusCount {
		if status != 200 && status != 404 && count > 0 {
			fmt.Printf("│   %-54s │\n",
				fmt.Sprintf("HTTP %d 错误: %d次", status, count))
		}
	}

	fmt.Printf("└%s┘\n", thickSeparator)
}

// 辅助函数：居中显示文本
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}

// 辅助函数：截断文本
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength-3] + "..."
}

// 安全的百分比计算函数
func safePercent(part, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(part) / float64(total) * 100
}

// TestSingleUserGetConcurrent 单用户查询接口并发测试
func TestSingleUserGetConcurrent(t *testing.T) {
	t.Run("单用户查询接口大并发压力测试", func(t *testing.T) {
		runBatchConcurrentTest(t, "单用户查询接口压力测试", testSingleUserGetRequest)
	})
}

// TestSingleUserGetCachePenetration 缓存击穿测试
func TestSingleUserGetCachePenetration(t *testing.T) {
	t.Run("缓存击穿测试", func(t *testing.T) {
		// 登录获取token
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		fmt.Printf("🔥 开始缓存击穿测试，使用用户ID: %s\n", CachePenetrationUserID)
		fmt.Printf("📊 并发用户: %d, 每用户请求: %d, 总请求: %d\n",
			CachePenetrationTestUsers, CachePenetrationRequests,
			CachePenetrationTestUsers*CachePenetrationRequests)

		apiPath := fmt.Sprintf(SingleUserPath, CachePenetrationUserID)
		var wg sync.WaitGroup
		stats := &PerformanceStats{
			StatusCount:       make(map[int]int),
			BusinessCodeCount: make(map[int]int),
		}

		startTime := time.Now()

		// 分批发送请求
		for batch := 0; batch < CachePenetrationTestUsers; batch++ {
			wg.Add(1)
			go func(batchID int) {
				defer wg.Done()

				for i := 0; i < CachePenetrationRequests; i++ {
					start := time.Now()
					resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
					duration := time.Since(start)

					mu.Lock()
					stats.TotalRequests++
					if err == nil && resp.HTTPStatus == http.StatusUnauthorized {
						stats.SuccessCount++
						stats.TotalDuration += duration
					} else {
						stats.UnexpectedFailCount++
					}
					stats.Durations = append(stats.Durations, duration)

					if resp != nil {
						stats.StatusCount[resp.HTTPStatus]++
						stats.BusinessCodeCount[resp.Code]++
					}
					mu.Unlock()

					// 记录数据库查询次数
					if resp != nil && resp.HTTPStatus == http.StatusNotFound {
						cachePenetrationMutex.Lock()
						cachePenetrationCounter++
						cachePenetrationMutex.Unlock()
					}

					time.Sleep(time.Duration(rand.IntN(50)) * time.Millisecond)
				}
			}(batch)
			time.Sleep(CachePenetrationBatchDelay)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		// 输出测试结果
		fmt.Printf("\n🔥 缓存击穿测试结果:\n")
		fmt.Printf("总请求数: %d\n", stats.TotalRequests)
		fmt.Printf("成功请求 (404): %d\n", stats.SuccessCount)
		fmt.Printf("失败请求: %d\n", stats.UnexpectedFailCount)
		fmt.Printf("总耗时: %v\n", totalDuration)
		fmt.Printf("平均QPS: %.1f\n", float64(stats.TotalRequests)/totalDuration.Seconds())
		fmt.Printf("数据库查询次数 (模拟): %d\n", cachePenetrationCounter)

		// 检查缓存击穿
		dbQueryRate := float64(cachePenetrationCounter) / float64(stats.TotalRequests)
		if dbQueryRate > 0.1 {
			fmt.Printf("❌ 可能发生缓存击穿，数据库查询比例: %.1f%%\n", dbQueryRate*100)
		} else {
			fmt.Printf("✅ 缓存击穿防护有效，数据库查询比例: %.1f%%\n", dbQueryRate*100)
		}
	})
}

// TestSingleUserGetEdgeCases 边界情况测试
func TestSingleUserGetEdgeCases(t *testing.T) {
	t.Run("单用户查询边界情况测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		testCases := []struct {
			name         string
			userID       string
			expectedHTTP int
			expectedBiz  int
			description  string
		}{
			{"空用户ID", "", http.StatusNotFound, RespCodeNotFound, "空字符串用户ID应该返回404"},
			{"超长用户ID", strings.Repeat("a", 1000), http.StatusBadRequest, RespCodeValidation, "超长用户ID应该返回400"},
			{"特殊字符用户ID", "user@#$%^&*()", http.StatusBadRequest, RespCodeValidation, "包含特殊字符的用户ID应该返回400"},
			{"SQL注入尝试", "user'; DROP TABLE users; --", http.StatusBadRequest, RespCodeValidation, "SQL注入尝试应该被拒绝并返回400"},
			{"数字用户ID", "1234567890", http.StatusOK, RespCodeSuccess, "纯数字用户ID应该正常处理"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				apiPath := fmt.Sprintf(SingleUserPath, tc.userID)
				resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)

				if err != nil {
					t.Fatalf("请求失败: %v", err)
				}

				if resp.HTTPStatus != tc.expectedHTTP {
					t.Errorf("HTTP状态码不符: 期望 %d, 实际 %d - %s", tc.expectedHTTP, resp.HTTPStatus, tc.description)
				}

				if resp.Code != tc.expectedBiz {
					t.Errorf("业务码不符: 期望 %d, 实际 %d - %s", tc.expectedBiz, resp.Code, tc.description)
				}

				t.Logf("测试通过: %s", tc.description)
			})
		}
	})
}

// TestSingleUserGetAuthentication 认证测试
func TestSingleUserGetAuthentication(t *testing.T) {
	t.Run("单用户查询认证测试", func(t *testing.T) {
		testCases := []struct {
			name         string
			token        string
			expectedHTTP int
			description  string
		}{
			{"无Token请求", "", http.StatusUnauthorized, "无Token应该返回401"},
			{"无效Token", "invalid-token-123456", http.StatusUnauthorized, "无效Token应该返回401"},
			{"过期Token", "expired-token-123456", http.StatusUnauthorized, "过期Token应该返回401"},
			{"格式错误Token", "Bearer-invalid-format", http.StatusUnauthorized, "格式错误Token应该返回401"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				apiPath := fmt.Sprintf(SingleUserPath, hotUserID)
				fullURL := ServerBaseURL + apiPath

				req, err := http.NewRequest(http.MethodGet, fullURL, nil)
				if err != nil {
					t.Fatalf("创建请求失败: %v", err)
				}

				if tc.token != "" {
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.token))
				}

				resp, err := httpClient.Do(req)
				if err != nil {
					t.Fatalf("请求失败: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expectedHTTP {
					t.Errorf("HTTP状态码不符: 期望 %d, 实际 %d - %s", tc.expectedHTTP, resp.StatusCode, tc.description)
				}

				t.Logf("测试通过: %s", tc.description)
			})
		}
	})
}

// TestSingleUserGetHotKey 热点Key测试
func TestSingleUserGetHotKey(t *testing.T) {
	t.Run("热点Key测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		fmt.Printf("🔥 开始热点Key测试，使用用户ID: %s\n", hotUserID)

		apiPath := fmt.Sprintf(SingleUserPath, hotUserID)
		var wg sync.WaitGroup
		stats := &PerformanceStats{
			StatusCount:       make(map[int]int),
			BusinessCodeCount: make(map[int]int),
		}

		startTime := time.Now()

		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				start := time.Now()
				resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
				duration := time.Since(start)

				mu.Lock()
				stats.TotalRequests++
				if err == nil && resp.HTTPStatus == http.StatusOK {
					stats.SuccessCount++
					stats.TotalDuration += duration
				} else {
					stats.UnexpectedFailCount++
				}
				stats.Durations = append(stats.Durations, duration)

				if resp != nil {
					stats.StatusCount[resp.HTTPStatus]++
					stats.BusinessCodeCount[resp.Code]++
				}
				mu.Unlock()

				time.Sleep(time.Duration(rand.IntN(10)) * time.Millisecond)
			}(i)
		}

		wg.Wait()
		totalDuration := time.Since(startTime)

		fmt.Printf("\n🔥 热点Key测试结果:\n")
		fmt.Printf("总请求数: %d\n", stats.TotalRequests)
		fmt.Printf("成功请求: %d\n", stats.SuccessCount)
		fmt.Printf("失败请求: %d\n", stats.UnexpectedFailCount)
		fmt.Printf("总耗时: %v\n", totalDuration)
		fmt.Printf("平均QPS: %.1f\n", float64(stats.TotalRequests)/totalDuration.Seconds())

		if stats.SuccessCount > 0 {
			avgResponseTime := stats.TotalDuration / time.Duration(stats.SuccessCount)
			fmt.Printf("平均响应时间: %v\n", avgResponseTime)
			if avgResponseTime > 100*time.Millisecond {
				fmt.Printf("❌ 热点Key处理较慢\n")
			} else {
				fmt.Printf("✅ 热点Key处理正常\n")
			}
		}
	})
}

// TestSingleUserGetColdStart 冷启动测试
func TestSingleUserGetColdStart(t *testing.T) {
	t.Run("冷启动测试", func(t *testing.T) {
		ctx, _, err := login(TestUsername, ValidPassword)
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}

		coldUserID := validUserIDs[len(validUserIDs)-1]
		fmt.Printf("❄️  开始冷启动测试，使用用户ID: %s\n", coldUserID)

		apiPath := fmt.Sprintf(SingleUserPath, coldUserID)

		// 第一次请求（冷启动）
		start := time.Now()
		resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
		coldStartDuration := time.Since(start)

		if err != nil {
			t.Fatalf("冷启动请求失败: %v", err)
		}

		if resp.HTTPStatus != http.StatusOK {
			t.Fatalf("冷启动请求失败，状态码: %d", resp.HTTPStatus)
		}

		// 第二次请求（预热后）
		start = time.Now()
		resp, err = sendTokenRequest(ctx, http.MethodGet, apiPath, nil)
		warmDuration := time.Since(start)

		if err != nil {
			t.Fatalf("预热后请求失败: %v", err)
		}

		fmt.Printf("❄️  冷启动测试结果:\n")
		fmt.Printf("冷启动耗时: %v\n", coldStartDuration)
		fmt.Printf("预热后耗时: %v\n", warmDuration)
		fmt.Printf("性能提升: %.1f%%\n", (float64(coldStartDuration)-float64(warmDuration))/float64(coldStartDuration)*100)

		if warmDuration < coldStartDuration/2 {
			fmt.Printf("✅ 缓存效果良好\n")
		} else {
			fmt.Printf("⚠️  缓存效果不明显\n")
		}
	})
}

// TestAllSingleUserGetTests 主测试函数
func TestAllSingleUserGetTests(t *testing.T) {
	t.Run("单用户查询完整测试套件", func(t *testing.T) {
		t.Run("边界情况测试", TestSingleUserGetEdgeCases)
		t.Run("认证测试", TestSingleUserGetAuthentication)
		t.Run("缓存击穿测试", TestSingleUserGetCachePenetration)
		t.Run("热点Key测试", TestSingleUserGetHotKey)
		t.Run("冷启动测试", TestSingleUserGetColdStart)
		t.Run("并发压力测试", TestSingleUserGetConcurrent)
	})
}

// LoginResponse 登录响应结构体
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	Expire       string `json:"expire"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	UserID       string `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
}
