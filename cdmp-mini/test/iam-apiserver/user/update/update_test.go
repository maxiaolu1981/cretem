/*
并发 Update 压力测试（每个并发用户独立登录并保存 token）
基于 create_test.go 的简化实现：
- 并发预取 token
- 每个并发用户对自身用户执行 PUT 更新
*/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"context"

	"golang.org/x/time/rate"
)

const (
	ServerBaseURL  = "http://192.168.10.8:8088"
	RequestTimeout = 30 * time.Second

	LoginAPIPath = "/login"
	UsersAPIPath = "/v1/users"

	RespCodeSuccess = 100001

	TestUsername = "admin"
	TestPassword = "Admin@2021"

	ConcurrentUsers = 10000
	RequestsPerUser = 100
	MaxConcurrent   = 1000
	BatchSize       = 50
	// success threshold percent for update operations (0-100)
	SuccessThresholdPercent = 90
)

var (
	httpClient   = createHTTPClient()
	userTokens   []string
	userExpiries []time.Time
	tokensMutex  sync.RWMutex
	// pre-created users for update target
	createdUsers    []string
	usernameCounter int64
	testRunID       = fmt.Sprintf("run_%d", time.Now().UnixNano())
	// stats
	createdSuccess      int32
	createdFail         int32
	tokenSuccess        int32
	tokenFail           int32
	updatesSuccess      int32
	updatesFail         int32
	updatesFailMu       sync.Mutex
	updatesFailByStatus = map[int]int{}
	// 全局限流器，限制所有请求速率
	limiter = rate.NewLimiter(rate.Limit(1000), 200) // 1000 QPS，突发200，可根据需要调整
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type APIResponse struct {
	HTTPStatus int         `json:"-"`
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Error      string      `json:"error,omitempty"`
	Data       interface{} `json:"data,omitempty"`
}

type UpdateUserRequest struct {
	Nickname string `json:"nickname,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
}

type UserMetadata struct {
	Name string `json:"name,omitempty"`
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

func TestUserUpdate_Concurrent(t *testing.T) {
	// 1. 获取 admin token 并并发预创建要更新的用户
	adminToken, err := getAuthTokenWithDebug()
	if err != nil {
		t.Fatalf("获取 admin token 失败: %v", err)
	}

	if err := preCreateTestUsersConcurrent(adminToken, ConcurrentUsers); err != nil {
		t.Fatalf("预创建用户失败: %v", err)
	}

	// 2. 我们已在 preCreateTestUsersConcurrent 中为每个创建的用户预取 token
	n := len(createdUsers)
	if n == 0 {
		t.Fatalf("没有可更新的用户")
	}

	// debug: print first few created usernames and tokens
	maxShow := 5
	if n < maxShow {
		maxShow = n
	}
	tokensMutex.RLock()
	fmt.Printf("示例用户（前 %d）：\n", maxShow)
	for i := 0; i < maxShow; i++ {
		tok := ""
		if i < len(userTokens) {
			tok = userTokens[i]
		}
		fmt.Printf("  idx=%d username=%s token_len=%d\n", i, createdUsers[i], len(tok))
	}
	tokensMutex.RUnlock()

	// Probe: verify the running server supports PUT /v1/users/:name
	// Use admin token to avoid permission issues
	probeUsername := createdUsers[0]
	probeURL := fmt.Sprintf(ServerBaseURL+UsersAPIPath+"/%s", probeUsername)
	probeBody := UpdateUserRequest{Nickname: "probe"}
	pb, _ := json.Marshal(probeBody)
	req, _ := http.NewRequest("PUT", probeURL, bytes.NewReader(pb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("探测 PUT 支持时请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("目标服务不支持 PUT 更新（返回404），探测用户=%s body=%s", probeUsername, string(body))
	}

	// 并发执行 update
	semaphore := make(chan struct{}, MaxConcurrent)
	var all sync.WaitGroup
	totalBatches := (n + BatchSize - 1) / BatchSize
	for batch := 0; batch < totalBatches; batch++ {
		start := batch * BatchSize
		end := start + BatchSize
		if end > n {
			end = n
		}
		for uid := start; uid < end; uid++ {
			all.Add(1)
			semaphore <- struct{}{}
			go func(id int) {
				defer all.Done()
				defer func() { <-semaphore }()
				for r := 0; r < RequestsPerUser; r++ {
					sendSingleUpdate(id, r)
					time.Sleep(50 * time.Millisecond)
				}
			}(uid)
		}
		// 短暂停顿
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
	}
	all.Wait()

	// final report
	totalUpdates := int64(atomic.LoadInt32(&updatesSuccess) + atomic.LoadInt32(&updatesFail))
	if totalUpdates == 0 {
		fmt.Printf("no update attempts were made\n")
		return
	}
	succ := int64(atomic.LoadInt32(&updatesSuccess))
	rate := float64(succ) * 100.0 / float64(totalUpdates)
	fmt.Printf("update stats: success=%d fail=%d total=%d success_rate=%.2f%%\n", succ, atomic.LoadInt32(&updatesFail), totalUpdates, rate)
	fmt.Printf("update failure by status: %v\n", updatesFailByStatus)
	if rate < float64(SuccessThresholdPercent) {
		t.Fatalf("更新成功率低于阈值 %d%%: %.2f%%", SuccessThresholdPercent, rate)
	}
}

// preCreateTestUsersConcurrent creates `count` users using the provided admin token.
func preCreateTestUsersConcurrent(adminToken string, count int) error {
	createdUsers = make([]string, 0, count)
	var createdMutex sync.Mutex
	sem := make(chan struct{}, 100)
	var wg sync.WaitGroup
	success := int32(0)
	for i := 0; i < count; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			username := generateTestUsernameFast(idx)
			reqBody := CreateUserRequest{
				Metadata: &UserMetadata{Name: username},
				Nickname: fmt.Sprintf("pre_%d", idx),
				Password: "Test@123456",
				Email:    fmt.Sprintf("%s@example.com", username),
				Status:   1,
				IsAdmin:  0,
			}
			// retry create a few times to improve chances under load
			created := false
			for attempt := 1; attempt <= 3; attempt++ {
				b, _ := json.Marshal(reqBody)
				req, _ := http.NewRequest("POST", ServerBaseURL+UsersAPIPath, bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					fmt.Printf("预创建请求失败 user=%s attempt=%d err=%v\n", username, attempt, err)
					atomic.AddInt32(&createdFail, 1)
					time.Sleep(time.Duration(attempt) * 150 * time.Millisecond)
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusCreated {
					createdMutex.Lock()
					createdUsers = append(createdUsers, username)
					createdMutex.Unlock()
					atomic.AddInt32(&success, 1)
					atomic.AddInt32(&createdSuccess, 1)
					created = true
					break
				}
				// non-201
				fmt.Printf("预创建失败 user=%s attempt=%d status=%d body=%s\n", username, attempt, resp.StatusCode, string(body))
				atomic.AddInt32(&createdFail, 1)
				time.Sleep(time.Duration(attempt) * 150 * time.Millisecond)
			}
			if !created {
				// give a small gap for observability
				// (we don't abort the whole run on some failures)
			}
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&success) == 0 {
		return fmt.Errorf("no users were created")
	}

	createdCount := int(atomic.LoadInt32(&success))
	fmt.Printf("预创建完成: requested=%d created=%d\n", count, createdCount)

	// short pause to allow server to fully persist users before login attempts
	time.Sleep(500 * time.Millisecond)

	// now prefetch per-user tokens by logging in as each created user (with retries)
	userTokens = make([]string, createdCount)
	userExpiries = make([]time.Time, createdCount)
	var tWg sync.WaitGroup
	for i := 0; i < createdCount; i++ {
		tWg.Add(1)
		go func(idx int) {
			defer tWg.Done()
			username := ""
			// access createdUsers in a safe way
			createdMutex.Lock()
			if idx < len(createdUsers) {
				username = createdUsers[idx]
			}
			createdMutex.Unlock()
			if username == "" {
				fmt.Printf("获取用户 token 失败 user index=%d: username empty\n", idx)
				atomic.AddInt32(&tokenFail, 1)
				return
			}
			var tok string
			var err error
			for attempt := 1; attempt <= 3; attempt++ {
				tok, err = getAuthTokenForUser(username, "Test@123456")
				if err == nil && tok != "" {
					atomic.AddInt32(&tokenSuccess, 1)
					tokensMutex.Lock()
					if idx < len(userTokens) {
						userTokens[idx] = tok
						userExpiries[idx] = time.Now().Add(30 * time.Minute)
					}
					tokensMutex.Unlock()
					return
				}
				fmt.Printf("获取用户 token 失败 user=%s attempt=%d: %v\n", username, attempt, err)
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			}
			// all attempts failed
			atomic.AddInt32(&tokenFail, 1)
		}(i)
	}
	tWg.Wait()

	// report how many tokens we obtained
	got := 0
	tokensMutex.RLock()
	for _, tk := range userTokens {
		if tk != "" {
			got++
		}
	}
	tokensMutex.RUnlock()
	fmt.Printf("token prefetch complete: tokens_obtained=%d/%d\n", got, createdCount)

	return nil
}

// getAuthTokenForUser logs in as the provided username/password and returns the access token.
func getAuthTokenForUser(username, password string) (string, error) {
	req := LoginRequest{Username: username, Password: password}
	b, _ := json.Marshal(req)
	resp, err := httpClient.Post(ServerBaseURL+LoginAPIPath, "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v", err)
	}
	if r.Code != RespCodeSuccess {
		return "", fmt.Errorf("登录失败: %s", string(body))
	}
	return r.Data.AccessToken, nil
}

func generateTestUsernameFast(index int) string {
	counter := atomic.AddInt64(&usernameCounter, 1)
	timestamp := time.Now().UnixNano() % 1000000
	base := fmt.Sprintf("test_%s_%d_%d", testRunID, index, timestamp+counter)
	if len(base) > 45 {
		return base[:45]
	}
	return base
}

func getAuthTokenWithDebug() (string, error) {
	loginReq := LoginRequest{Username: TestUsername, Password: TestPassword}
	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("登录请求序列化失败: %v", err)
	}
	resp, err := httpClient.Post(ServerBaseURL+LoginAPIPath, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var response struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v", err)
	}
	if response.Code != RespCodeSuccess {
		return "", fmt.Errorf("登录失败: %s", string(body))
	}
	if response.Data.AccessToken == "" {
		return "", fmt.Errorf("access_token为空")
	}
	return response.Data.AccessToken, nil
}

func sendSingleUpdate(userID, requestID int) {
	// 限流：每次请求前等待令牌
	_ = limiter.Wait(context.Background())

	// 使用用户自己的 token
	if userID < 0 || userID >= len(createdUsers) {
		fmt.Printf("invalid userID %d\n", userID)
		return
	}
	token := ""
	tokensMutex.RLock()
	if userID < len(userTokens) {
		token = userTokens[userID]
	}
	tokensMutex.RUnlock()
	if token == "" {
		fmt.Printf("user %d token empty\n", userID)
		return
	}

	// 使用实际创建的用户名
	username := createdUsers[userID]
	update := UpdateUserRequest{
		Nickname: fmt.Sprintf("up_%d_%d", userID, requestID),
		Email:    fmt.Sprintf("up_%d_%d@example.com", userID, requestID),
	}
	jsonData, _ := json.Marshal(update)
	url := fmt.Sprintf(ServerBaseURL+UsersAPIPath+"/%s", username)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(jsonData))
	if err != nil {
		fmt.Printf("构建请求失败: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("发送请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var apiResp APIResponse
	_ = json.Unmarshal(body, &apiResp)
	if resp.StatusCode == http.StatusOK && apiResp.Code == RespCodeSuccess {
		atomic.AddInt32(&updatesSuccess, 1)
		return
	}

	// failure path
	atomic.AddInt32(&updatesFail, 1)
	fmt.Printf("update failed user=%d status=%d body=%s\n", userID, resp.StatusCode, string(body))
	updatesFailMu.Lock()
	updatesFailByStatus[resp.StatusCode] = updatesFailByStatus[resp.StatusCode] + 1
	updatesFailMu.Unlock()

	if resp.StatusCode == http.StatusUnauthorized {
		// 清空 token
		tokensMutex.Lock()
		if userID >= 0 && userID < len(userTokens) {
			userTokens[userID] = ""
		}
		tokensMutex.Unlock()
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
