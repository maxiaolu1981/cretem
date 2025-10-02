/*
真正并发压力测试：并发创建用户 + 并发删除用户
*/
package deleteforce

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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"
)

// ==================== 压力测试配置常量 ====================
const (
	ServerBaseURL  = "http://localhost:8088"
	RequestTimeout = 30 * time.Second

	LoginAPIPath    = "/login"
	UsersAPIPath    = "/v1/users"
	ForceDeletePath = "/v1/users/%s/force"

	RespCodeSuccess = 100001

	// 测试账号
	TestUsername = "admin"
	TestPassword = "Admin@2021"

	// 创建用户并发配置
	PreCreateConcurrent = 1000 // 预创建并发数
	PreCreateBatchSize  = 1000 // 进度显示批次
	PreCreateTimeout    = 10 * time.Second

	// 删除用户并发配置
	ConcurrentDeleters = 1  // 并发删除器数量
	DeletesPerUser     = 10 // 每个删除器执行的删除次数
	MaxConcurrent      = 1  // 最大并发数
	BatchSize          = 1  // 批次大小
	PreCreateUsers     = 10 // 预先创建的用户数量

	// 模式配置
	DeleteModeRandom = "random"
)

// ==================== 数据结构 ====================
type DeleteTestResult struct {
	DeleterID   int
	RequestID   int
	Success     bool
	Duration    time.Duration
	Error       string
	DeletedUser string
}

type CreateTestResult struct {
	CreatorID int
	UserIndex int
	Success   bool
	Duration  time.Duration
	Error     string
	Username  string
}

type UserInfo struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Status   int    `json:"status"`
}

type PreCreateResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    UserInfo `json:"data"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

// ==================== 全局变量 ====================
var (
	httpClient  = createOptimizedHTTPClient()
	statsMutex  sync.RWMutex
	globalToken string
	tokenMutex  sync.RWMutex
	tokenExpiry time.Time
)

// 统计变量
var (
	// 创建统计
	totalCreateRequests int64
	createSuccessCount  int64
	createFailCount     int64
	totalCreateDuration time.Duration
	createErrorResults  []CreateTestResult

	// 删除统计
	totalDeleteRequests int64
	deleteSuccessCount  int64
	deleteFailCount     int64
	totalDeleteDuration time.Duration
	deleteErrorResults  []DeleteTestResult

	// 用户管理
	availableUsers  []string
	usersMutex      sync.RWMutex
	usernameCounter int64
)

// ==================== 主测试函数 ====================
func TestUserForceDelete_RealConcurrent(t *testing.T) {
	// 初始化环境
	checkResourceLimits()
	setHigherFileLimit()

	width := getTerminalWidth()
	printHeader("🚀 开始并发创建+删除用户压力测试", width)

	// 1. 获取认证Token
	fmt.Printf("🔑 获取认证Token...\n")
	token, err := getAuthTokenWithDebug()
	if err != nil {
		fmt.Printf("❌ 获取Token失败: %v\n", err)
		return
	}

	tokenMutex.Lock()
	globalToken = token
	tokenExpiry = time.Now().Add(30 * time.Minute)
	tokenMutex.Unlock()

	fmt.Printf("✅ 成功获取Token: %s...\n", token[:min(20, len(token))])

	// 2. 并发预创建测试用户
	fmt.Printf("👥 并发预创建测试用户...\n")
	createStartTime := time.Now()
	if err := preCreateTestUsersConcurrent(token, PreCreateUsers); err != nil {
		fmt.Printf("❌ 预创建用户失败: %v\n", err)
		return
	}
	createDuration := time.Since(createStartTime)
	fmt.Printf("✅ 并发创建完成，耗时: %v\n", createDuration.Round(time.Millisecond))

	// 3. 显示测试配置
	totalExpectedDeletes := ConcurrentDeleters * DeletesPerUser
	fmt.Printf("📊 删除测试配置:\n")
	fmt.Printf("  ├─ 并发删除器数: %d\n", ConcurrentDeleters)
	fmt.Printf("  ├─ 每删除器操作数: %d\n", DeletesPerUser)
	fmt.Printf("  ├─ 总删除操作数: %d\n", totalExpectedDeletes)
	fmt.Printf("  ├─ 预创建用户数: %d\n", PreCreateUsers)
	fmt.Printf("  ├─ 实际成功创建: %d\n", createSuccessCount)
	fmt.Printf("  ├─ 创建失败数: %d\n", createFailCount)
	fmt.Printf("  ├─ 创建耗时: %v\n", createDuration.Round(time.Millisecond))
	fmt.Printf("  ├─ 删除模式: %s\n", DeleteModeRandom)
	fmt.Printf("  ├─ 最大并发数: %d\n", MaxConcurrent)
	fmt.Printf("  └─ 使用Token认证: 是\n")
	fmt.Printf("%s\n", strings.Repeat("─", width))

	// 4. 执行并发删除测试
	deleteStartTime := time.Now()

	// 启动实时统计显示
	stopStats := startDeleteRealTimeStats(deleteStartTime)
	defer stopStats()

	executeConcurrentDeleteTest()

	// 5. 输出最终结果
	deleteDuration := time.Since(deleteStartTime)
	printFinalResults(createDuration, deleteDuration, width)

	// 6. 数据校验
	validateResults(width)
}

// ==================== 并发预创建用户 ====================
func preCreateTestUsersConcurrent(token string, count int) error {
	availableUsers = make([]string, 0, count)

	// 重置统计
	atomic.StoreInt64(&totalCreateRequests, 0)
	atomic.StoreInt64(&createSuccessCount, 0)
	atomic.StoreInt64(&createFailCount, 0)
	totalCreateDuration = 0
	createErrorResults = make([]CreateTestResult, 0)

	concurrentCreators := PreCreateConcurrent
	if count < concurrentCreators {
		concurrentCreators = count
	}

	// ✅ 移除未使用的 semaphore 和 wg 变量声明

	var mutex sync.Mutex

	successCount := int32(0)
	failedCount := int32(0)

	fmt.Printf("🚀 开始并发创建 %d 个用户，并发数: %d\n", count, concurrentCreators)
	startTime := time.Now()

	// 使用工作池模式
	jobs := make(chan int, count)
	results := make(chan bool, count)

	// 启动worker
	for w := 0; w < concurrentCreators; w++ {
		go createUserWorker(w, token, jobs, results, &mutex)
	}

	// 分发任务
	go func() {
		for i := 0; i < count; i++ {
			jobs <- i
		}
		close(jobs)
	}()

	// 收集结果
	go func() {
		for i := 0; i < count; i++ {
			success := <-results
			if success {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failedCount, 1)
			}

			// 进度显示
			if (i+1)%PreCreateBatchSize == 0 {
				elapsed := time.Since(startTime)
				currentTotal := int32(i + 1)
				rate := float64(currentTotal) / elapsed.Seconds()
				fmt.Printf("   进度: %d/%d, 成功: %d, 失败: %d, 速度: %.1f 用户/秒\n",
					currentTotal, count, successCount, failedCount, rate)
			}
		}
	}()

	// 等待所有任务完成
	for atomic.LoadInt32(&successCount)+atomic.LoadInt32(&failedCount) < int32(count) {
		time.Sleep(100 * time.Millisecond)
	}

	totalTime := time.Since(startTime)
	fmt.Printf("✅ 并发创建完成: 成功 %d, 失败 %d, 总耗时: %v, 平均速度: %.1f 用户/秒\n",
		successCount, failedCount, totalTime.Round(time.Second),
		float64(successCount)/totalTime.Seconds())

	return nil
}

// 创建用户的工作线程
func createUserWorker(workerID int, token string, jobs <-chan int, results chan<- bool, mutex *sync.Mutex) {
	for index := range jobs {
		results <- createSingleUser(workerID, token, index, mutex)
	}
}

// 创建单个用户
func createSingleUser(workerID int, token string, index int, mutex *sync.Mutex) bool {
	start := time.Now()
	username := generateTestUsernameFast(index)

	userReq := CreateUserRequest{
		Metadata: &UserMetadata{Name: username},
		Nickname: fmt.Sprintf("压力测试用户%d", index),
		Password: "Test@123456",
		Email:    fmt.Sprintf("stress%d@test.com", index),
		Phone:    fmt.Sprintf("138%08d", index),
		Status:   1,
		IsAdmin:  0,
	}

	jsonData, err := json.Marshal(userReq)
	if err != nil {
		recordCreateResult(workerID, index, false, time.Since(start), "JSON序列化失败", username)
		return false
	}

	req, err := http.NewRequest("POST", ServerBaseURL+UsersAPIPath, bytes.NewReader(jsonData))
	if err != nil {
		recordCreateResult(workerID, index, false, time.Since(start), "创建请求失败", username)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// 使用单独的HTTP客户端避免超时影响其他请求
	client := &http.Client{Timeout: PreCreateTimeout}
	resp, err := client.Do(req)
	if err != nil {
		recordCreateResult(workerID, index, false, time.Since(start),
			fmt.Sprintf("请求发送失败: %v", err), username)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordCreateResult(workerID, index, false, time.Since(start), "响应读取失败", username)
		return false
	}

	if resp.StatusCode != http.StatusCreated {
		recordCreateResult(workerID, index, false, time.Since(start),
			fmt.Sprintf("HTTP状态码错误: %d", resp.StatusCode), username)
		return false
	}

	var apiResp PreCreateResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		recordCreateResult(workerID, index, false, time.Since(start), "响应解析失败", username)
		return false
	}

	if apiResp.Code != RespCodeSuccess {
		recordCreateResult(workerID, index, false, time.Since(start),
			fmt.Sprintf("业务创建失败: %s", apiResp.Message), username)
		return false
	}

	// 成功创建，添加到可用列表
	mutex.Lock()
	availableUsers = append(availableUsers, username)
	mutex.Unlock()

	recordCreateResult(workerID, index, true, time.Since(start), "", username)
	return true
}

// ==================== 并发删除用户 ====================
func executeConcurrentDeleteTest() {
	semaphore := make(chan struct{}, MaxConcurrent)
	var wg sync.WaitGroup

	totalBatches := (ConcurrentDeleters + BatchSize - 1) / BatchSize

	for batch := 0; batch < totalBatches; batch++ {
		batchStart := batch * BatchSize
		batchEnd := min((batch+1)*BatchSize, ConcurrentDeleters)

		fmt.Printf("🔄 处理删除批次 %d/%d: 删除器 %d-%d\n",
			batch+1, totalBatches, batchStart, batchEnd-1)

		var batchWg sync.WaitGroup
		for deleterID := batchStart; deleterID < batchEnd; deleterID++ {
			batchWg.Add(1)
			go func(did int) {
				defer batchWg.Done()
				sendDeleteRequests(did, semaphore, &wg)
			}(deleterID)
		}
		batchWg.Wait()

		if batch < totalBatches-1 {
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		}
	}

	wg.Wait()
}

func sendDeleteRequests(deleterID int, semaphore chan struct{}, wg *sync.WaitGroup) {
	var deleterWg sync.WaitGroup

	for requestID := 0; requestID < DeletesPerUser; requestID++ {
		wg.Add(1)
		deleterWg.Add(1)
		semaphore <- struct{}{}

		go func(did, rid int) {
			defer wg.Done()
			defer deleterWg.Done()
			defer func() { <-semaphore }()

			sendSingleDeleteRequest(did, rid)
		}(deleterID, requestID)

		// 控制请求间隔
		time.Sleep(100 * time.Microsecond)
	}

	deleterWg.Wait()
}

func sendSingleDeleteRequest(deleterID, requestID int) {
	start := time.Now()

	// 获取有效Token
	token := getValidToken()
	if token == "" {
		recordDeleteResult(deleterID, requestID, false, time.Since(start), "Token获取失败", "")
		return
	}

	// 选择要删除的用户
	username := selectUserForDeletion()
	if username == "" {
		recordDeleteResult(deleterID, requestID, false, time.Since(start), "无可用用户可删除", "")
		return
	}

	// 构建删除URL
	deleteURL := fmt.Sprintf(ServerBaseURL+ForceDeletePath, username)

	// 创建DELETE请求
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		recordDeleteResult(deleterID, requestID, false, time.Since(start), "创建删除请求失败", username)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)

	// 发送删除请求
	resp, err := httpClient.Do(req)
	if err != nil {
		recordDeleteResult(deleterID, requestID, false, time.Since(start),
			fmt.Sprintf("删除请求发送失败: %v", err), username)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordDeleteResult(deleterID, requestID, false, time.Since(start), "响应读取失败", username)
		return
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		recordDeleteResult(deleterID, requestID, false, time.Since(start), "响应解析失败", username)
		return
	}

	duration := time.Since(start)

	// 判断删除成功条件
	success := false
	errorMsg := ""

	switch {
	case resp.StatusCode == http.StatusOK && apiResp.Code == RespCodeSuccess:
		success = true
	case resp.StatusCode == http.StatusNotFound:
		errorMsg = fmt.Sprintf("用户不存在: %s", username)
	case resp.StatusCode == http.StatusUnauthorized:
		errorMsg = "权限认证失败"
		tokenMutex.Lock()
		globalToken = ""
		tokenMutex.Unlock()
	default:
		errorMsg = fmt.Sprintf("HTTP=%d, Code=%d, Msg=%s",
			resp.StatusCode, apiResp.Code, apiResp.Message)
	}

	if success {
		recordDeleteResult(deleterID, requestID, true, duration, "", username)
		// 从可用用户列表中移除已删除的用户
		removeDeletedUser(username)
	} else {
		recordDeleteResult(deleterID, requestID, false, duration, errorMsg, username)
	}
}

// ==================== 工具函数 ====================
func selectUserForDeletion() string {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	if len(availableUsers) == 0 {
		return ""
	}

	// 随机选择用户进行删除
	index := rand.IntN(len(availableUsers))
	user := availableUsers[index]

	// 立即移除，避免并发竞争
	availableUsers = append(availableUsers[:index], availableUsers[index+1:]...)

	return user
}

func removeDeletedUser(username string) {
	// 这个函数现在在 selectUserForDeletion 中已经处理了移除逻辑
	// 保留这个函数是为了兼容性，实际可能不再需要
}

// 快速用户名生成
func generateTestUsernameFast(index int) string {
	timestamp := time.Now().UnixNano() % 1000000
	counter := atomic.AddInt64(&usernameCounter, 1)
	return fmt.Sprintf("test_%d_%d_%d", index, timestamp, counter)
}

// 优化的HTTP客户端
func createOptimizedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   50,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			DisableKeepAlives:     false,
		},
	}
}

// ==================== 统计记录函数 ====================
func recordCreateResult(creatorID, userIndex int, success bool, duration time.Duration, errorMsg, username string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	totalCreateRequests++

	if success {
		createSuccessCount++
		totalCreateDuration += duration
	} else {
		createFailCount++
		createErrorResults = append(createErrorResults, CreateTestResult{
			CreatorID: creatorID,
			UserIndex: userIndex,
			Success:   success,
			Duration:  duration,
			Error:     errorMsg,
			Username:  username,
		})
	}
}

func recordDeleteResult(deleterID, requestID int, success bool, duration time.Duration, errorMsg, username string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	totalDeleteRequests++

	if success {
		deleteSuccessCount++
		totalDeleteDuration += duration
	} else {
		deleteFailCount++
		deleteErrorResults = append(deleteErrorResults, DeleteTestResult{
			DeleterID:   deleterID,
			RequestID:   requestID,
			Success:     success,
			Duration:    duration,
			Error:       errorMsg,
			DeletedUser: username,
		})
	}
}

// ==================== 结果显示函数 ====================
func startDeleteRealTimeStats(startTime time.Time) func() {
	ticker := time.NewTicker(500 * time.Millisecond)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				statsMutex.RLock()
				currentTotal := totalDeleteRequests
				currentSuccess := deleteSuccessCount
				currentFail := deleteFailCount
				remainingUsers := len(availableUsers)
				statsMutex.RUnlock()

				if currentTotal == 0 {
					continue
				}

				duration := time.Since(startTime)
				qps := float64(currentTotal) / duration.Seconds()
				successRate := float64(currentSuccess) / float64(currentTotal) * 100

				fmt.Printf("\r📈 删除实时统计: 操作=%d, 成功=%d, 失败=%d, 剩余用户=%d, QPS=%.1f, 成功率=%.1f%%, 耗时=%v",
					currentTotal, currentSuccess, currentFail, remainingUsers, qps, successRate, duration.Round(time.Second))

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

func printFinalResults(createDuration, deleteDuration time.Duration, width int) {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	fmt.Printf("\n\n🎯 并发创建+删除压力测试完成!\n")
	fmt.Printf("%s\n", strings.Repeat("═", width))

	// 创建统计
	createSuccessRate := float64(createSuccessCount) / float64(totalCreateRequests) * 100
	createQPS := float64(totalCreateRequests) / createDuration.Seconds()

	fmt.Printf("📊 创建性能统计:\n")
	fmt.Printf("  ├─ 总创建操作: %d\n", totalCreateRequests)
	fmt.Printf("  ├─ 成功创建: %d\n", createSuccessCount)
	fmt.Printf("  ├─ 创建失败: %d\n", createFailCount)
	fmt.Printf("  ├─ 创建成功率: %.2f%%\n", createSuccessRate)
	fmt.Printf("  ├─ 总耗时: %v\n", createDuration.Round(time.Millisecond))
	fmt.Printf("  ├─ 平均QPS: %.1f\n", createQPS)

	if createSuccessCount > 0 {
		avgCreateDuration := totalCreateDuration / time.Duration(createSuccessCount)
		fmt.Printf("  └─ 平均响应时间: %v\n", avgCreateDuration.Round(time.Millisecond))
	}

	// 删除统计
	deleteSuccessRate := float64(deleteSuccessCount) / float64(totalDeleteRequests) * 100
	deleteQPS := float64(totalDeleteRequests) / deleteDuration.Seconds()

	fmt.Printf("\n📊 删除性能统计:\n")
	fmt.Printf("  ├─ 总删除操作: %d\n", totalDeleteRequests)
	fmt.Printf("  ├─ 成功删除: %d\n", deleteSuccessCount)
	fmt.Printf("  ├─ 删除失败: %d\n", deleteFailCount)
	fmt.Printf("  ├─ 删除成功率: %.2f%%\n", deleteSuccessRate)
	fmt.Printf("  ├─ 剩余用户数: %d\n", len(availableUsers))
	fmt.Printf("  ├─ 总耗时: %v\n", deleteDuration.Round(time.Millisecond))
	fmt.Printf("  ├─ 平均QPS: %.1f\n", deleteQPS)

	if deleteSuccessCount > 0 {
		avgDeleteDuration := totalDeleteDuration / time.Duration(deleteSuccessCount)
		fmt.Printf("  └─ 平均响应时间: %v\n", avgDeleteDuration.Round(time.Millisecond))
	}

	// 错误分析
	if len(deleteErrorResults) > 0 {
		fmt.Printf("\n🔍 删除错误分析 (前10个):\n")
		displayErrors := min(10, len(deleteErrorResults))
		for i := 0; i < displayErrors; i++ {
			err := deleteErrorResults[i]
			fmt.Printf("  %d. 删除器%d-请求%d [用户:%s]: %s (耗时: %v)\n",
				i+1, err.DeleterID, err.RequestID, err.DeletedUser, err.Error, err.Duration.Round(time.Millisecond))
		}
	}

	fmt.Printf("%s\n", strings.Repeat("═", width))
}

func validateResults(width int) {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	expectedDeletes := ConcurrentDeleters * DeletesPerUser
	if int(totalDeleteRequests) != expectedDeletes {
		fmt.Printf("⚠️  统计警告: 实际删除操作数(%d) != 预期操作数(%d)\n", totalDeleteRequests, expectedDeletes)
	}

	if len(availableUsers) > 0 {
		fmt.Printf("⚠️  剩余用户警告: 还有 %d 个用户未被删除\n", len(availableUsers))
	}
}

// ==================== 其他工具函数 ====================
func getAuthTokenWithDebug() (string, error) {
	loginReq := LoginRequest{
		Username: TestUsername,
		Password: TestPassword,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("登录请求序列化失败: %v", err)
	}

	resp, err := httpClient.Post(ServerBaseURL+LoginAPIPath, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取登录响应失败: %v", err)
	}

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

	if response.Code != RespCodeSuccess {
		return "", fmt.Errorf("登录失败: %s", response.Message)
	}

	if response.Data.AccessToken == "" {
		return "", fmt.Errorf("access_token为空")
	}

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

func printHeader(title string, width int) {
	fmt.Printf("\n%s\n", strings.Repeat("═", width))
	fmt.Printf("%s\n", title)
	fmt.Printf("%s\n", strings.Repeat("═", width))
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
