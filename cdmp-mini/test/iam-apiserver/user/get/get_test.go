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
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
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
	ServerBaseURL  = "http://localhost:8080"
	RequestTimeout = 30 * time.Second

	LoginAPIPath   = "/login"
	SingleUserPath = "/v1/users/%s"

	TestUsername  = "admin"
	ValidPassword = "Admin@2021"

	RespCodeSuccess    = 100001
	RespCodeNotFound   = 110001 // 根据实际系统调整为110001
	RespCodeForbidden  = 110009 //无权访问
	RespCodeValidation = 100400

	ConcurrentUsers       = 1000
	RequestsPerUser       = 1000
	RequestInterval       = 50 * time.Millisecond
	BatchSize             = 1000
	HotUserRequestPercent = 50 // 热点用户请求百分比
	InvalidRequestPercent = 20 //无效请求百分比

	P50Threshold   = 50 * time.Millisecond
	P90Threshold   = 100 * time.Millisecond
	P99Threshold   = 200 * time.Millisecond
	ErrorRateLimit = 0.01

	// 缓存击穿测试相关常量
	CachePenetrationTestUsers  = 100                      // 缓存击穿测试并发用户数
	CachePenetrationRequests   = 1000                     // 每个用户请求次数
	CachePenetrationUserID     = "nonexistent-user-12345" // 用于缓存击穿测试的用户ID
	CachePenetrationBatchDelay = 100 * time.Millisecond   // 批次间延迟
)

// 在全局变量部分添加预定义的用户列表
var (
	// 预定义的有效用户列表
	predefinedValidUsers = []string{
		"admin",
		"load_test_user0",
		"load_test_user1",
		"load_test_user10",
		"load_test_user11",
		"load_test_user12",
		"load_test_user13",
		"load_test_user14",
		"load_test_user15",
		"load_test_user16",
		"load_test_user17",
		"load_test_user18",
		"load_test_user19",
		"load_test_user2",
		"load_test_user20",
		"load_test_user21",
		"load_test_user22",
		"load_test_user23",
		"load_test_user24",
		"load_test_user25",
		"load_test_user26",
		"load_test_user27",
		"load_test_user28",
		"load_test_user29",
		"load_test_user3",
		"load_test_user30",
		"load_test_user31",
		"load_test_user32",
		"load_test_user33",
		"load_test_user34",
		"load_test_user35",
		"load_test_user36",
		"load_test_user37",
		"load_test_user38",
		"load_test_user39",
		"load_test_user4",
		"load_test_user40",
		"load_test_user41",
		"load_test_user42",
		"load_test_user43",
		"load_test_user44",
		"load_test_user45",
		"load_test_user46",
		"load_test_user47",
		"load_test_user48",
		"load_test_user49",
		"load_test_user5",
		"load_test_user6",
		"load_test_user7",
		"load_test_user8",
		"load_test_user9",
		"retry_test_user",
		"retry_test_user1",
		"retry_test_user2",
		"user_0_1001_372873",
		"user_0_1004_729522",
		"user_0_1008_40490",
		"user_0_1010_976539",
		"user_0_1015_168368",
		"user_0_1016_568163",
		"user_0_1019_670840",
		"user_0_1027_710751",
		"user_0_1027_921182",
		"user_0_102_595108",
		"user_0_1031_418599",
		"user_0_1032_609978",
		"user_0_1042_186955",
		"user_0_1042_342750",
		"user_0_1042_704641",
		"user_0_1045_253993",
		"user_0_1045_375405",
		"user_0_1045_932427",
		"user_0_1049_191240",
		"user_0_104_183952",
		"user_0_1051_468825",
		"user_0_1053_115290",
		"user_0_1055_626532",
		"user_0_1057_393401",
		"user_0_1057_487584",
		"user_0_105_563522",
		"user_0_105_854544",
		"user_0_1060_18629",
		"user_0_1060_82786",
		"user_0_1060_853609",
		"user_0_1066_569808",
		"user_0_1066_629549",
		"user_0_1067_997672",
		"user_0_1068_650401",
		"user_0_1070_977069",
		"user_0_1072_324189",
		"user_0_1073_283199",
		"user_0_1073_391458",
		"user_0_1076_764703",
		"user_0_1078_365884",
		"user_0_1078_653618",
		"user_0_1079_314004",
	}

	// 预定义的热点用户
	predefinedHotUser = "admin"

	// 预定义的无权限用户
	predefinedUnauthorizedUser = "test_user_123"

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
	httpClient = createHTTPClient()
	mu         sync.Mutex
	statsMu    sync.Mutex

	validUserIDs     []string
	hotUserID        string
	invalidUserIDs   []string
	unauthorizedUser string

	cachePenetrationCounter int
	cachePenetrationMutex   sync.Mutex
)

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

func TestMain(m *testing.M) {
	fmt.Println("初始化单用户查询接口测试环境...")
	initTestData()

	checkResourceLimits()
	setHigherFileLimit()

	// 登录并验证用户
	ctx, _, err := login(TestUsername, ValidPassword)
	if err == nil {
		verifyTestUsers(ctx)
	} else {
		fmt.Printf("⚠️  登录失败，跳过用户验证: %v\n", err)
	}

	code := m.Run()
	os.Exit(code)
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
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		// 如果解析失败，说明不是标准格式，尝试直接解析为token数据
		return parseDirectLoginResponse(resp, respBody, username)
	}

	apiResp.HTTPStatus = resp.StatusCode

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
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, err
	}
	apiResp.HTTPStatus = resp.StatusCode

	return &apiResp, nil
}

// ==================== 辅助函数 ====================
func verifyTestUsers(ctx *TestContext) {
	fmt.Printf("🔍 验证测试用户是否存在...\n")

	testUsers := []string{
		predefinedHotUser,
		predefinedUnauthorizedUser,
		validUserIDs[1],
		validUserIDs[10],
		invalidUserIDs[0],
	}

	for _, user := range testUsers {
		apiPath := fmt.Sprintf(SingleUserPath, user)
		resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)

		if err != nil {
			fmt.Printf("   ❌ %s: 请求失败 - %v\n", user, err)
		} else if resp.HTTPStatus == http.StatusOK {
			fmt.Printf("   ✅ %s: 存在 (HTTP %d)\n", user, resp.HTTPStatus)
		} else {
			fmt.Printf("   ℹ️  %s: 不存在 (HTTP %d, 业务码 %d)\n", user, resp.HTTPStatus, resp.Code)
		}

		time.Sleep(10 * time.Millisecond)
	}
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

func monitorMemoryUsage(stop chan bool) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("\033[22;1H\033[K")
			fmt.Printf("💾 内存使用: Alloc=%.1fMB, TotalAlloc=%.1fMB, Sys=%.1fMB, Goroutines=%d, GC次数=%d",
				float64(m.Alloc)/1024/1024,
				float64(m.TotalAlloc)/1024/1024,
				float64(m.Sys)/1024/1024,
				runtime.NumGoroutine(),
				m.NumGC)
		case <-stop:
			return
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkPerformanceMetric(metricName string, actual, threshold time.Duration) {
	if actual <= threshold {
		fmt.Printf("✅ %s: %v (<= %v)\n", metricName, actual, threshold)
	} else {
		fmt.Printf("❌ %s: %v (> %v)\n", metricName, actual, threshold)
	}
}

// ==================== 测试用例函数 ====================

// TestSingleUserGetConcurrent 单用户查询接口并发测试
func TestSingleUserGetConcurrent(t *testing.T) {
	t.Run("单用户查询接口大并发压力测试", func(t *testing.T) {
		runBatchConcurrentTest(t, "单用户查询接口压力测试", testSingleUserGetRequest)
	})
}

func testSingleUserGetRequest(t *testing.T, userID int, ctx *TestContext) (bool, bool, *APIResponse, int, int) {
	var targetUserID string
	var expectedHTTP int
	var expectedBiz int
	isExpectedFailure := false

	// 随机决定请求类型
	randNum := rand.IntN(100)
	switch {
	case randNum < HotUserRequestPercent:
		targetUserID = hotUserID
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
	case randNum < HotUserRequestPercent+InvalidRequestPercent:
		targetUserID = invalidUserIDs[rand.IntN(len(invalidUserIDs))]
		expectedHTTP = http.StatusNotFound
		expectedBiz = RespCodeNotFound
		isExpectedFailure = true
	case randNum < HotUserRequestPercent+InvalidRequestPercent+10:
		targetUserID = unauthorizedUser
		expectedHTTP = http.StatusForbidden
		expectedBiz = RespCodeForbidden
		isExpectedFailure = true
	default:
		targetUserID = validUserIDs[rand.IntN(len(validUserIDs))]
		expectedHTTP = http.StatusOK
		expectedBiz = RespCodeSuccess
	}

	apiPath := fmt.Sprintf(SingleUserPath, targetUserID)

	resp, err := sendTokenRequest(ctx, http.MethodGet, apiPath, nil)

	if err != nil {
		t.Logf("请求失败: %v", err)
		return false, false, nil, expectedHTTP, expectedBiz
	}

	// ========== 正确的验证逻辑 ==========
	success := (resp.HTTPStatus == expectedHTTP) && (resp.Code == expectedBiz)

	// 关键修正：如果验证失败，就不是预期失败
	if !success {
		isExpectedFailure = false
	}
	// ===================================

	// 使用更简洁但完整的日志格式
	log.Warnf("调试: 用户=%-15s 类型=%-4s 期望HTTP=%-3d 实际HTTP=%-3d 成功=%-5v",
		truncateString(targetUserID, 15),
		getShortRequestType(randNum),
		expectedHTTP, resp.HTTPStatus,
		success)
	return success, isExpectedFailure, resp, expectedHTTP, expectedBiz
}

// 辅助函数：截断字符串
func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length]
}

// 辅助函数：简化的请求类型
func getShortRequestType(randNum int) string {
	switch {
	case randNum < HotUserRequestPercent:
		return "热点"
	case randNum < HotUserRequestPercent+InvalidRequestPercent:
		return "无效"
	case randNum < HotUserRequestPercent+InvalidRequestPercent+10:
		return "权限"
	default:
		return "随机"
	}
}

// ==================== 并发测试框架 ====================
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize
	totalStats := &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}

	// 先登录获取token
	ctx, _, err := login(TestUsername, ValidPassword)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		batchStats := runConcurrentTest(t, testName, startUser, endUser, ctx, testFunc)

		// 合并统计信息
		statsMu.Lock()
		totalStats.TotalRequests += batchStats.TotalRequests
		totalStats.SuccessCount += batchStats.SuccessCount
		totalStats.ExpectedFailCount += batchStats.ExpectedFailCount
		totalStats.UnexpectedFailCount += batchStats.UnexpectedFailCount
		totalStats.TotalDuration += batchStats.TotalDuration
		totalStats.Durations = append(totalStats.Durations, batchStats.Durations...)

		for status, count := range batchStats.StatusCount {
			totalStats.StatusCount[status] += count
		}
		for code, count := range batchStats.BusinessCodeCount {
			totalStats.BusinessCodeCount[code] += count
		}
		statsMu.Unlock()

		// 批次间休息
		if batch < totalBatches-1 {
			fmt.Printf("⏸️  批次间休息 2秒...\n")
			time.Sleep(2 * time.Second)
			runtime.GC()
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 输出性能报告
	printPerformanceReport(totalStats, testName)
}

func runConcurrentTest(t *testing.T, testName string, startUser, endUser int, ctx *TestContext, testFunc func(*testing.T, int, *TestContext) (bool, bool, *APIResponse, int, int)) *PerformanceStats {
	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			width = w
		}
	}

	fmt.Print("\033[2J")
	fmt.Print("\033[1;1H")

	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("🚀 开始并发测试: %s\n", testName)
	fmt.Printf("📊 并发用户: %d-%d, 每用户请求: %d, 总请求: %d\n",
		startUser, endUser-1, RequestsPerUser, (endUser-startUser)*RequestsPerUser)
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()
	var wg sync.WaitGroup
	stats := &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}

	progress := make(chan string, 100)
	done := make(chan bool)

	stopMonitor := make(chan bool)
	go monitorMemoryUsage(stopMonitor)

	// 进度显示协程
	go func() {
		line := 6
		for msg := range progress {
			fmt.Printf("\033[%d;1H", line)
			fmt.Printf("\033[K")
			fmt.Printf("   %s", msg)
			line++
			if line > 10 {
				line = 6
			}
		}
		done <- true
	}()

	// 统计信息显示协程
	statsTicker := time.NewTicker(500 * time.Millisecond)
	defer statsTicker.Stop()

	go func() {
		for range statsTicker.C {
			mu.Lock()
			currentSuccess := stats.SuccessCount
			currentExpectedFail := stats.ExpectedFailCount
			currentUnexpectedFail := stats.UnexpectedFailCount
			currentDuration := time.Since(startTime)
			totalRequests := currentSuccess + currentExpectedFail + currentUnexpectedFail
			mu.Unlock()

			if totalRequests > 0 {
				fmt.Printf("\033[12;1H\033[K")
				fmt.Printf("   ✅ 成功请求: %d\n", currentSuccess)
				fmt.Printf("\033[13;1H\033[K")
				fmt.Printf("   🟡 预期失败: %d\n", currentExpectedFail)
				fmt.Printf("\033[14;1H\033[K")
				fmt.Printf("   🔴 意外失败: %d\n", currentUnexpectedFail)
				fmt.Printf("\033[15;1H\033[K")
				fmt.Printf("   📈 成功率: %.1f%%\n", float64(currentSuccess)/float64(totalRequests)*100)
				fmt.Printf("\033[16;1H\033[K")
				fmt.Printf("   ⏱️  当前耗时: %v\n", currentDuration.Round(time.Millisecond))
				fmt.Printf("\033[17;1H\033[K")
				fmt.Printf("   🚀 实时QPS: %.1f\n", float64(totalRequests)/currentDuration.Seconds())
				if stats.TotalDuration > 0 && stats.SuccessCount > 0 {
					fmt.Printf("\033[18;1H\033[K")
					fmt.Printf("   ⚡ 平均耗时: %v\n", stats.TotalDuration/time.Duration(stats.SuccessCount))
				}
				fmt.Printf("\033[19;1H\033[K")
				fmt.Printf("%s", strings.Repeat("─", width))
			}
		}
	}()

	// 启动并发测试
	for i := startUser; i < endUser; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			for j := 0; j < RequestsPerUser; j++ {
				progress <- fmt.Sprintf("🟡 [用户%d] 请求 %d 开始...", userID, j+1)

				start := time.Now()
				success, isExpectedFailure, resp, _, _ := testFunc(t, userID, ctx)
				duration := time.Since(start)

				mu.Lock()
				stats.TotalRequests++
				if success {
					if isExpectedFailure {
						// 预期失败的成功请求（如正确的404响应）
						stats.ExpectedFailCount++
						progress <- fmt.Sprintf("🟡 [用户%d] 请求 %d 预期失败 (耗时: %v)", userID, j+1, duration)
					} else {
						// 真正的成功请求
						stats.SuccessCount++
						stats.TotalDuration += duration
						progress <- fmt.Sprintf("🟢 [用户%d] 请求 %d 成功 (耗时: %v)", userID, j+1, duration)
					}
				} else {
					if isExpectedFailure {
						// 预期失败的请求但响应不符合预期
						stats.ExpectedFailCount++
						progress <- fmt.Sprintf("🟡 [用户%d] 请求 %d 预期失败但响应异常 (耗时: %v)", userID, j+1, duration)
					} else {
						// 真正的意外失败
						stats.UnexpectedFailCount++
						progress <- fmt.Sprintf("🔴 [用户%d] 请求 %d 意外失败 (耗时: %v)", userID, j+1, duration)
					}
				}
				stats.Durations = append(stats.Durations, duration)

				if resp != nil {
					stats.StatusCount[resp.HTTPStatus]++
					stats.BusinessCodeCount[resp.Code]++
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
	stopMonitor <- true

	duration := time.Since(startTime)
	totalRequests := (endUser - startUser) * RequestsPerUser

	fmt.Printf("\033[20;1H\033[K")
	fmt.Printf("%s\n", strings.Repeat("═", width))
	fmt.Printf("📊 批次测试完成!\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))
	fmt.Printf("   ✅ 总成功数: %d/%d (%.1f%%)\n", stats.SuccessCount, totalRequests, float64(stats.SuccessCount)/float64(totalRequests)*100)
	fmt.Printf("   🟡 预期失败: %d/%d (%.1f%%)\n", stats.ExpectedFailCount, totalRequests, float64(stats.ExpectedFailCount)/float64(totalRequests)*100)
	fmt.Printf("   🔴 意外失败: %d/%d (%.1f%%)\n", stats.UnexpectedFailCount, totalRequests, float64(stats.UnexpectedFailCount)/float64(totalRequests)*100)
	fmt.Printf("   ⏱️  总耗时: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("   🚀 平均QPS: %.1f\n", float64(totalRequests)/duration.Seconds())
	if stats.SuccessCount > 0 {
		fmt.Printf("   ⚡ 平均响应时间: %v\n", stats.TotalDuration/time.Duration(stats.SuccessCount))
	}
	fmt.Printf("%s\n", strings.Repeat("═", width))

	runtime.GC()
	return stats
}

// ==================== 性能报告函数 ====================
func printPerformanceReport(stats *PerformanceStats, testName string) {
	if len(stats.Durations) == 0 {
		fmt.Println("⚠️  无性能数据可分析")
		return
	}

	// 计算分位值
	sort.Slice(stats.Durations, func(i, j int) bool {
		return stats.Durations[i] < stats.Durations[j]
	})

	p50 := stats.Durations[int(float64(len(stats.Durations))*0.5)]
	p90 := stats.Durations[int(float64(len(stats.Durations))*0.9)]
	p99 := stats.Durations[int(float64(len(stats.Durations))*0.99)]

	fmt.Printf("\n📊 %s 性能报告\n", testName)
	fmt.Printf("═%s═\n", strings.Repeat("═", 50))
	fmt.Printf("总请求数: %d\n", stats.TotalRequests)
	fmt.Printf("成功数: %d (%.2f%%)\n", stats.SuccessCount, float64(stats.SuccessCount)/float64(stats.TotalRequests)*100)
	fmt.Printf("预期失败数: %d (%.2f%%)\n", stats.ExpectedFailCount, float64(stats.ExpectedFailCount)/float64(stats.TotalRequests)*100)
	fmt.Printf("意外失败数: %d (%.2f%%)\n", stats.UnexpectedFailCount, float64(stats.UnexpectedFailCount)/float64(stats.TotalRequests)*100)

	// 避免除零错误
	if stats.TotalDuration > 0 {
		qps := float64(stats.TotalRequests) / stats.TotalDuration.Seconds()
		if !math.IsInf(qps, 0) && !math.IsNaN(qps) {
			fmt.Printf("平均QPS: %.1f\n", qps)
		} else {
			fmt.Printf("平均QPS: 无法计算\n")
		}
	} else {
		fmt.Printf("平均QPS: 无法计算\n")
	}

	fmt.Printf("P50响应时间: %v\n", p50)
	fmt.Printf("P90响应时间: %v\n", p90)
	fmt.Printf("P99响应时间: %v\n", p99)

	// 避免除零错误
	if stats.SuccessCount > 0 {
		avgResponseTime := stats.TotalDuration / time.Duration(stats.SuccessCount)
		fmt.Printf("平均响应时间: %v\n", avgResponseTime)
	} else {
		fmt.Printf("平均响应时间: 无成功请求\n")
	}

	// 检查性能指标
	fmt.Printf("\n📈 性能指标检查:\n")
	checkPerformanceMetric("P50", p50, P50Threshold)
	checkPerformanceMetric("P90", p90, P90Threshold)
	checkPerformanceMetric("P99", p99, P99Threshold)

	// 计算真实错误率（只包含意外失败）
	realErrorRate := float64(stats.UnexpectedFailCount) / float64(stats.TotalRequests)
	if realErrorRate <= ErrorRateLimit {
		fmt.Printf("✅ 真实错误率: %.4f%% (<= %.2f%%)\n", realErrorRate*100, ErrorRateLimit*100)
	} else {
		fmt.Printf("❌ 真实错误率: %.4f%% (> %.2f%%)\n", realErrorRate*100, ErrorRateLimit*100)
	}

	// 输出状态码分布
	fmt.Printf("\n🌐 HTTP状态码分布:\n")
	for status, count := range stats.StatusCount {
		fmt.Printf("  %d: %d (%.1f%%)\n", status, count, float64(count)/float64(stats.TotalRequests)*100)
	}

	// 输出业务码分布
	fmt.Printf("\n📋 业务码分布:\n")
	for code, count := range stats.BusinessCodeCount {
		fmt.Printf("  %d: %d (%.1f%%)\n", code, count, float64(count)/float64(stats.TotalRequests)*100)
	}
	// 添加详细的错误分析
	fmt.Printf("\n🔍 错误分析:\n")
	fmt.Printf("  预期404但标记为意外失败: %d次\n", stats.StatusCount[404]-stats.ExpectedFailCount)
	fmt.Printf("  真正的意外错误: %d次\n", stats.UnexpectedFailCount)

	// 按状态码分析
	for status, count := range stats.StatusCount {
		if status != 200 && status != 404 {
			fmt.Printf("  HTTP %d 错误: %d次\n", status, count)
		}
	}
}

// ==================== 其他测试函数 ====================

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
					if err == nil && resp.HTTPStatus == http.StatusNotFound {
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
