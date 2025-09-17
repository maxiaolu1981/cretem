package main

import (
	"encoding/json"
	"fmt"
	"io"
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

	redisV8 "github.com/go-redis/redis/v8"
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
	RespCodeNotFound   = 100404
	RespCodeForbidden  = 100403
	RespCodeValidation = 100400

	ConcurrentUsers       = 1000
	RequestsPerUser       = 200
	RequestInterval       = 50 * time.Millisecond
	BatchSize             = 200
	HotUserRequestPercent = 50
	InvalidRequestPercent = 20

	P50Threshold   = 50 * time.Millisecond
	P90Threshold   = 100 * time.Millisecond
	P99Threshold   = 200 * time.Millisecond
	ErrorRateLimit = 0.01
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
	UserID       string
}

type PerformanceStats struct {
	TotalRequests     int
	SuccessCount      int
	FailCount         int
	TotalDuration     time.Duration
	Durations         []time.Duration
	StatusCount       map[int]int
	BusinessCodeCount map[int]int
}

// ==================== 全局变量 ====================
var (
	httpClient  = createHTTPClient()
	redisClient *redisV8.Client
	mu          sync.Mutex
	statsMu     sync.Mutex

	validUserIDs     []string
	hotUserID        string
	invalidUserIDs   []string
	unauthorizedUser string

	globalStats = &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}
)

// ==================== 初始化函数 ====================
func initTestData() {
	// 生成有效的用户ID
	validUserIDs = make([]string, 1000)
	for i := 0; i < 1000; i++ {
		validUserIDs[i] = fmt.Sprintf("user-%08d", i+1)
	}

	// 设置热点用户
	hotUserID = validUserIDs[0]

	// 生成无效的用户ID
	invalidUserIDs = make([]string, 200)
	for i := 0; i < 200; i++ {
		invalidUserIDs[i] = fmt.Sprintf("invalid-%08d", i+1)
	}

	// 设置无权限访问的用户
	unauthorizedUser = "admin-user-001"
}

func TestMain(m *testing.M) {
	fmt.Println("初始化单用户查询接口测试环境...")
	initTestData()

	checkResourceLimits()
	setHigherFileLimit()

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

// ==================== 并发测试框架 ====================
func runBatchConcurrentTest(t *testing.T, testName string, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) {
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize
	totalStats := &PerformanceStats{
		StatusCount:       make(map[int]int),
		BusinessCodeCount: make(map[int]int),
	}

	for batch := 0; batch < totalBatches; batch++ {
		startUser := batch * BatchSize
		endUser := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("\n🔄 执行第 %d/%d 批测试: 用户 %d-%d\n",
			batch+1, totalBatches, startUser, endUser-1)

		batchStats := runConcurrentTest(t, testName, startUser, endUser, testFunc)

		// 合并统计信息
		statsMu.Lock()
		totalStats.TotalRequests += batchStats.TotalRequests
		totalStats.SuccessCount += batchStats.SuccessCount
		totalStats.FailCount += batchStats.FailCount
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

func runConcurrentTest(t *testing.T, testName string, startUser, endUser int, testFunc func(*testing.T, int, string, string) (bool, *APIResponse, int, int)) *PerformanceStats {
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
			currentFail := stats.FailCount
			currentDuration := time.Since(startTime)
			totalRequests := currentSuccess + currentFail
			mu.Unlock()

			if totalRequests > 0 {
				fmt.Printf("\033[12;1H\033[K")
				fmt.Printf("   ✅ 成功请求: %d\n", currentSuccess)
				fmt.Printf("\033[13;1H\033[K")
				fmt.Printf("   ❌ 失败请求: %d\n", currentFail)
				fmt.Printf("\033[14;1H\033[K")
				fmt.Printf("   📈 成功率: %.1f%%\n", float64(currentSuccess)/float64(totalRequests)*100)
				fmt.Printf("\033[15;1H\033[K")
				fmt.Printf("   ⏱️  当前耗时: %v\n", currentDuration.Round(time.Millisecond))
				fmt.Printf("\033[16;1H\033[K")
				fmt.Printf("   🚀 实时QPS: %.1f\n", float64(totalRequests)/currentDuration.Seconds())
				if stats.TotalDuration > 0 && stats.SuccessCount > 0 {
					fmt.Printf("\033[17;1H\033[K")
					fmt.Printf("   ⚡ 平均耗时: %v\n", stats.TotalDuration/time.Duration(stats.SuccessCount))
				}
				fmt.Printf("\033[18;1H\033[K")
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
				//requestID := userID*RequestsPerUser + j + 1
				progress <- fmt.Sprintf("🟡 [用户%d] 请求 %d 开始...", userID, j+1)

				start := time.Now()
				success, resp, _, _ := testFunc(t, userID, username, password)
				duration := time.Since(start)

				mu.Lock()
				stats.TotalRequests++
				if success {
					stats.SuccessCount++
					stats.TotalDuration += duration
				} else {
					stats.FailCount++
				}
				stats.Durations = append(stats.Durations, duration)

				if resp != nil {
					stats.StatusCount[resp.HTTPStatus]++
					stats.BusinessCodeCount[resp.Code]++
				}

				if success {
					progress <- fmt.Sprintf("🟢 [用户%d] 请求 %d 成功 (耗时: %v)", userID, j+1, duration)
				} else {
					progress <- fmt.Sprintf("🔴 [用户%d] 请求 %d 失败 (耗时: %v)", userID, j+1, duration)
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
	fmt.Printf("   ❌ 总失败数: %d/%d (%.1f%%)\n", stats.FailCount, totalRequests, float64(stats.FailCount)/float64(totalRequests)*100)
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
	fmt.Printf("失败数: %d (%.2f%%)\n", stats.FailCount, float64(stats.FailCount)/float64(stats.TotalRequests)*100)
	fmt.Printf("总耗时: %v\n", stats.TotalDuration)
	fmt.Printf("平均QPS: %.1f\n", float64(stats.TotalRequests)/stats.TotalDuration.Seconds())
	fmt.Printf("P50响应时间: %v\n", p50)
	fmt.Printf("P90响应时间: %v\n", p90)
	fmt.Printf("P99响应时间: %v\n", p99)
	fmt.Printf("平均响应时间: %v\n", stats.TotalDuration/time.Duration(stats.SuccessCount))

	// 检查性能指标
	fmt.Printf("\n📈 性能指标检查:\n")
	checkPerformanceMetric("P50", p50, P50Threshold)
	checkPerformanceMetric("P90", p90, P90Threshold)
	checkPerformanceMetric("P99", p99, P99Threshold)

	errorRate := float64(stats.FailCount) / float64(stats.TotalRequests)
	if errorRate <= ErrorRateLimit {
		fmt.Printf("✅ 错误率: %.4f%% (<= %.2f%%)\n", errorRate*100, ErrorRateLimit*100)
	} else {
		fmt.Printf("❌ 错误率: %.4f%% (> %.2f%%)\n", errorRate*100, ErrorRateLimit*100)
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
}

func checkPerformanceMetric(metricName string, actual, threshold time.Duration) {
	if actual <= threshold {
		fmt.Printf("✅ %s: %v (<= %v)\n", metricName, actual, threshold)
	} else {
		fmt.Printf("❌ %s: %v (> %v)\n", metricName, actual, threshold)
	}
}

// ==================== 系统资源函数 ====================
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

// ==================== 辅助函数 ====================
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func generateValidUserName(userID int) string {
	timestamp := time.Now().UnixNano() % 10000
	return fmt.Sprintf("user_%d_%d_%d", userID, timestamp, rand.IntN(1000000))
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
