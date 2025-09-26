/*
真正并发压力测试：移除Token认证，实现高并发
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

	UsersAPIPath = "/v1/users"

	RespCodeSuccess = 100001

	// 真正并发配置
	ConcurrentUsers = 10000 // 并发用户数
	RequestsPerUser = 20    // 每用户请求数
	MaxConcurrent   = 10000 // 最大并发数控制
	BatchSize       = 100   // 批次大小
)

// ==================== 数据结构 ====================
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
	httpClient = createHTTPClient()
	statsMutex sync.RWMutex
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
	printHeader("🚀 开始真正并发压力测试", width)

	// 测试配置
	totalExpectedRequests := ConcurrentUsers * RequestsPerUser
	fmt.Printf("📊 测试配置:\n")
	fmt.Printf("  ├─ 并发用户数: %d\n", ConcurrentUsers)
	fmt.Printf("  ├─ 每用户请求数: %d\n", RequestsPerUser)
	fmt.Printf("  ├─ 总请求数: %d\n", totalExpectedRequests)
	fmt.Printf("  ├─ 最大并发数: %d\n", MaxConcurrent)
	fmt.Printf("  └─ 预期QPS: 10,000+\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	startTime := time.Now()

	// 启动实时统计显示
	stopStats := startRealTimeStats(startTime)
	defer stopStats()

	// 执行真正并发测试
	executeRealConcurrentTest()

	// 输出最终结果
	duration := time.Since(startTime)
	printFinalResults(duration, width)

	// 数据校验
	validateResults(width)
}

// ==================== 核心并发逻辑 ====================
func executeRealConcurrentTest() {
	// 控制并发的信号量
	semaphore := make(chan struct{}, MaxConcurrent)
	var wg sync.WaitGroup

	// 分批处理避免内存爆炸
	totalBatches := (ConcurrentUsers + BatchSize - 1) / BatchSize

	for batch := 0; batch < totalBatches; batch++ {
		batchStart := batch * BatchSize
		batchEnd := min((batch+1)*BatchSize, ConcurrentUsers)

		fmt.Printf("🔄 处理批次 %d/%d: 用户 %d-%d\n",
			batch+1, totalBatches, batchStart, batchEnd-1)

		// 批次内并发
		var batchWg sync.WaitGroup
		for userID := batchStart; userID < batchEnd; userID++ {
			batchWg.Add(1)
			go func(uid int) {
				defer batchWg.Done()
				sendUserRequests(uid, semaphore, &wg)
			}(userID)
		}
		batchWg.Wait()

		// 批次间休息，释放资源
		if batch < totalBatches-1 {
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		}
	}

	wg.Wait()
}

// 发送单个用户的所有请求
func sendUserRequests(userID int, semaphore chan struct{}, wg *sync.WaitGroup) {
	// 每个用户的请求并发发送
	var userWg sync.WaitGroup

	for requestID := 0; requestID < RequestsPerUser; requestID++ {
		wg.Add(1)
		userWg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(uid, rid int) {
			defer wg.Done()
			defer userWg.Done()
			defer func() { <-semaphore }() // 释放信号量

			sendSingleRequest(uid, rid)
		}(userID, requestID)

		// 控制请求启动节奏（微秒级）
		time.Sleep(50 * time.Microsecond)
	}

	userWg.Wait()
}

// 发送单个请求（无Token认证）
func sendSingleRequest(userID, requestID int) {
	start := time.Now()

	// 1. 准备请求数据
	userReq := CreateUserRequest{
		Metadata: &UserMetadata{
			Name: generateUniqueUsername(userID, requestID),
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

	// 2. 发送HTTP请求（无Token认证）
	resp, err := httpClient.Post(ServerBaseURL+UsersAPIPath, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), fmt.Sprintf("请求发送失败: %v", err))
		return
	}
	defer resp.Body.Close()

	// 3. 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordResult(userID, requestID, false, time.Since(start), "响应读取失败")
		return
	}

	// 4. 解析响应
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		recordResult(userID, requestID, false, time.Since(start), "响应解析失败")
		return
	}
	apiResp.HTTPStatus = resp.StatusCode

	// 5. 验证结果
	duration := time.Since(start)
	success := resp.StatusCode == http.StatusCreated && apiResp.Code == RespCodeSuccess

	if success {
		recordResult(userID, requestID, true, duration, "")
	} else {
		recordResult(userID, requestID, false, duration,
			fmt.Sprintf("HTTP=%d, Code=%d, Msg=%s", resp.StatusCode, apiResp.Code, apiResp.Message))
	}
}

// ==================== 统计和监控 ====================
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

// ==================== 工具函数 ====================
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

func generateUniqueUsername(userID, requestID int) string {
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

	// 基础统计
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

	// 性能评估
	fmt.Printf("\n🏆 性能评估:\n")
	switch {
	case qps >= 5000:
		fmt.Printf("  💚 优秀: QPS > 5000 (高性能)\n")
	case qps >= 2000:
		fmt.Printf("  💛 良好: QPS > 2000\n")
	case qps >= 1000:
		fmt.Printf("  🟡 一般: QPS > 1000\n")
	case qps >= 500:
		fmt.Printf("  🟠 及格: QPS > 500\n")
	default:
		fmt.Printf("  🔴 较差: QPS < 500\n")
	}

	// 错误分析
	if len(errorResults) > 0 {
		fmt.Printf("\n🔍 错误分析 (前10个):\n")
		displayErrors := min(10, len(errorResults))
		for i := 0; i < displayErrors; i++ {
			err := errorResults[i]
			fmt.Printf("  %d. 用户%d-请求%d: %s (耗时: %v)\n",
				i+1, err.UserID, err.RequestID, err.Error, err.Duration.Round(time.Millisecond))
		}
		if len(errorResults) > displayErrors {
			fmt.Printf("  ... 还有%d个错误未显示\n", len(errorResults)-displayErrors)
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

	if totalRequests != successCount+failCount {
		fmt.Printf("⚠️  统计警告: 总请求数(%d) != 成功数(%d) + 失败数(%d)\n",
			totalRequests, successCount, failCount)
	}
}

func checkResourceLimits() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		fmt.Printf("📁 文件描述符限制: Soft=%d, Hard=%d\n", rLimit.Cur, rLimit.Max)
		if rLimit.Cur < 10000 {
			fmt.Printf("⚠️  建议设置: ulimit -n 10000\n")
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

// ==================== 测试主函数 ====================
func TestMain(m *testing.M) {
	fmt.Println("🛠️ 初始化压力测试环境...")
	code := m.Run()

	// 清理资源
	if transport, ok := httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	os.Exit(code)
}
