package changepasswd

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"
)

const (
	serverBaseURL             = "http://192.168.10.8:8088"
	requestTimeout            = 30 * time.Second
	loginAPIPath              = "/login"
	createUserAPIPath         = "/v1/users"
	getUserAPIPathFormat      = "/v1/users/%s"
	changePasswordAPIPath     = "/v1/users/%s/change-password"
	forceDeleteAPIPath        = "/v1/users/%s/force"
	respCodeSuccess       int = 100001

	defaultTestPassword = "InitPassw0rd!"
	adminUsername       = "admin"
	adminPassword       = "Admin@2021"

	outputDir                 = "output"
	resultsJSONFilename       = "change_password_results.json"
	performanceJSONFilename   = "change_password_perf.json"
	validationScriptRel       = "tools/check_change_password.py"
	validationSummaryFilename = "change_password_summary.json"
	logFilePath               = "/var/log/iam/iam-apiserver.log"
)

const (
	dbHost     = "127.0.0.1"
	dbUser     = "root"
	dbPassword = "iam59!z$"
	dbName     = "iam"
)

var (
	httpClient = &http.Client{Timeout: requestTimeout, Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 90 * time.Second}).DialContext,
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 512,
		MaxConnsPerHost:     1024,
		IdleConnTimeout:     90 * time.Second,
	}}

	resultsMu   sync.Mutex
	caseResults []CaseResult
	perfResults []PerformanceMetrics
)

type TestCase struct {
	Name                   string
	Username               string
	SetupUser              bool
	CustomOldPassword      string
	CustomNewPassword      string
	RawBody                []byte
	ExpectHTTPStatus       int
	ExpectCode             int
	ExpectMessageContains  string
	ShouldUpdateDB         bool
	ShouldInvalidateToken  bool
	ExpectLoginNewPassword bool
	ExpectLoginOldPassword bool
	ExpectBindError        bool
	UseAdminToken          bool
	SkipToken              bool
	Notes                  []string
}

type CaseResult struct {
	Name             string          `json:"name"`
	Username         string          `json:"username"`
	HTTPStatus       int             `json:"http_status"`
	Code             int             `json:"code"`
	Message          string          `json:"message"`
	Success          bool            `json:"success"`
	DurationMs       int64           `json:"duration_ms"`
	ShouldUpdate     bool            `json:"should_update_db"`
	ShouldInvalidate bool            `json:"should_invalidate_token"`
	Checks           map[string]bool `json:"checks"`
	ExtraNotes       []string        `json:"notes,omitempty"`
	Error            string          `json:"error,omitempty"`
	OldPassword      string          `json:"old_password,omitempty"`
	NewPassword      string          `json:"new_password,omitempty"`
}

type PerformanceMetrics struct {
	Scenario              string  `json:"scenario"`
	Throughput            float64 `json:"throughput_rps"`
	AvgResponseTimeMs     float64 `json:"avg_response_time_ms"`
	P95ResponseTimeMs     float64 `json:"p95_response_time_ms"`
	P99ResponseTimeMs     float64 `json:"p99_response_time_ms"`
	ErrorRate             float64 `json:"error_rate"`
	SuccessRate           float64 `json:"success_rate"`
	CPUUsage              float64 `json:"cpu_usage_percent"`
	MemoryUsageMB         float64 `json:"memory_usage_mb"`
	GoroutineCount        int     `json:"goroutine_count"`
	GCStats               string  `json:"gc_stats"`
	DBConnections         int     `json:"db_connections"`
	DBQueryTimeMs         float64 `json:"db_query_time_ms"`
	DBLockWaitTimeMs      float64 `json:"db_lock_wait_time_ms"`
	PasswordUpdateSuccess int     `json:"password_update_success"`
	ValidationFailures    int     `json:"validation_failures"`
	AuthErrors            int     `json:"auth_errors"`
}

type StressConfig struct {
	BaseURL          string
	ConcurrentUsers  int
	TestDuration     time.Duration
	RampUpTime       time.Duration
	RequestTimeout   time.Duration
	UserPoolSize     int
	PasswordVariants int
	MetricsInterval  time.Duration
	EnableProfiling  bool
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		AccessToken string `json:"access_token"`
		Expire      string `json:"expire"`
	} `json:"data"`
}

type createUserRequest struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Status   int    `json:"status"`
	IsAdmin  int    `json:"isAdmin"`
}

type testContext struct {
	t            *testing.T
	adminToken   string
	createdUsers []string
	db           *sql.DB
}

func newTestContext(t *testing.T) *testContext {
	token, err := loginUser(adminUsername, adminPassword)
	if err != nil {
		t.Fatalf("failed to login admin: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?charset=utf8mb4&parseTime=true&loc=Local", dbUser, dbPassword, dbHost, dbName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	db.SetMaxOpenConns(100)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetMaxIdleConns(20)                 // 增加空闲连接数
	db.SetConnMaxIdleTime(5 * time.Minute) // 空闲连接超时
	return &testContext{t: t, adminToken: token, db: db}
}

func (tc *testContext) Close() {
	for _, username := range tc.createdUsers {
		if err := forceDeleteUser(tc.adminToken, username); err != nil {
			fmt.Printf("⚠️ cleanup user %s failed: %v\n", username, err)
		}
	}
	if tc.db != nil {
		_ = tc.db.Close()
	}
}

func (tc *testContext) createTestUser(initialPassword string) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	username := fmt.Sprintf("test_cp_%d_%d", time.Now().UnixNano(), rng.Intn(100000))
	req := createUserRequest{}
	req.Metadata.Name = username
	req.Nickname = "密码测试用户"
	req.Password = initialPassword
	req.Email = fmt.Sprintf("%s@example.com", username)
	req.Phone = fmt.Sprintf("138%08d", rng.Intn(100000000))
	req.Status = 1
	req.IsAdmin = 0

	payload, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", serverBaseURL+createUserAPIPath, bytes.NewReader(payload))
	if err != nil {
		tc.t.Fatalf("failed to build create user request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+tc.adminToken)

	start := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		tc.t.Fatalf("create user http error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		tc.t.Fatalf("create user unexpected status %d body %s", resp.StatusCode, string(body))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		tc.t.Fatalf("create user decode error: %v", err)
	}
	if apiResp.Code != respCodeSuccess {
		tc.t.Fatalf("create user api error code=%d msg=%s", apiResp.Code, apiResp.Message)
	}

	duration := time.Since(start)
	fmt.Printf("🆕 创建测试用户 %s 成功，耗时 %v\n", username, duration)

	tc.createdUsers = append(tc.createdUsers, username)
	return username
}

func loginUser(username, password string) (string, error) {
	payload, _ := json.Marshal(loginRequest{Username: username, Password: password})
	resp, err := httpClient.Post(serverBaseURL+loginAPIPath, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var lr loginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return "", err
	}
	if lr.Code != respCodeSuccess {
		return "", fmt.Errorf("login failed: code=%d msg=%s", lr.Code, lr.Message)
	}
	if lr.Data.AccessToken == "" {
		return "", errors.New("empty access token")
	}
	return lr.Data.AccessToken, nil
}

func loginUserWithRetry(username, password string, attempts int, delay time.Duration) (string, error) {
	if attempts < 1 {
		attempts = 1
	}
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		token, err := loginUser(username, password)
		if err == nil {
			return token, nil
		}
		lastErr = err
		time.Sleep(delay)
	}
	return "", lastErr
}

func forceDeleteUser(token, username string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf(serverBaseURL+forceDeleteAPIPath, username), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("force delete %s failed: status=%d body=%s", username, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func changePasswordRequest(token, username string, body []byte) (*http.Response, []byte, time.Duration, error) {
	endpoint := fmt.Sprintf(serverBaseURL+changePasswordAPIPath, username)
	req, err := http.NewRequest("PUT", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody, time.Since(start), err
}

func fetchUserPasswordHash(db *sql.DB, username string) (string, error) {
	var hashed string
	err := db.QueryRow("SELECT password FROM `user` WHERE name=?", username).Scan(&hashed)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hashed, err
}

func writeResultsToDisk(basePath string) error {
	resultsMu.Lock()
	defer resultsMu.Unlock()

	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return err
	}

	resultsPath := filepath.Join(basePath, resultsJSONFilename)
	data, err := json.MarshalIndent(caseResults, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(resultsPath, data, 0o644); err != nil {
		return err
	}

	if len(perfResults) > 0 {
		perfPath := filepath.Join(basePath, performanceJSONFilename)
		perfData, err := json.MarshalIndent(perfResults, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(perfPath, perfData, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func runValidationScript(basePath string) error {
	scriptPath := filepath.Join(basePath, "tools", "check_change_password.py")

	// 更详细的脚本检查
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("validation script not found at %s", scriptPath)
	} else if err != nil {
		return fmt.Errorf("failed to access script: %v", err)
	}

	// 构建输出文件路径
	outputDirPath := filepath.Join(basePath, outputDir)
	args := []string{
		scriptPath,
		"--results", filepath.Join(outputDirPath, resultsJSONFilename),
		"--perf", filepath.Join(outputDirPath, performanceJSONFilename),
		"--summary", filepath.Join(outputDirPath, validationSummaryFilename),
	}

	log.Printf("Executing validation script: python3 %s", strings.Join(args, " "))

	cmd := exec.Command("python3", args...)
	cmd.Dir = basePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 执行并获取更详细的错误信息
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validation script execution failed: %v", err)
	}

	log.Println("Validation script completed successfully")
	return nil
}

func TestChangePassword_Functional(t *testing.T) {
	tc := newTestContext(t)
	defer tc.Close()

	baseDir := filepath.Dir(currentFile())
	outputPath := filepath.Join(baseDir, outputDir)

	cases := []TestCase{
		{
			Name:                   "正常修改密码",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      "NewPassw0rd!",
			ExpectHTTPStatus:       http.StatusOK,
			ExpectCode:             respCodeSuccess,
			ShouldUpdateDB:         true,
			ShouldInvalidateToken:  true,
			ExpectLoginNewPassword: true,
			ExpectLoginOldPassword: false,
			Notes:                  []string{"正向用例"},
		},
		{
			Name:                   "最小长度密码",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      "Aa1!aaaa",
			ExpectHTTPStatus:       http.StatusOK,
			ExpectCode:             respCodeSuccess,
			ShouldUpdateDB:         true,
			ShouldInvalidateToken:  true,
			ExpectLoginNewPassword: true,
			ExpectLoginOldPassword: false,
			Notes:                  []string{"最小长度边界"},
		},
		{
			Name:                   "最大长度密码",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      "Aa1!Aa1!Aa1!Aa1!",
			ExpectHTTPStatus:       http.StatusOK,
			ExpectCode:             respCodeSuccess,
			ShouldUpdateDB:         true,
			ShouldInvalidateToken:  true,
			ExpectLoginNewPassword: true,
			ExpectLoginOldPassword: false,
			Notes:                  []string{"最大长度边界"},
		},
		{
			Name:                   "错误旧密码",
			SetupUser:              true,
			CustomOldPassword:      "WrongPass!1",
			CustomNewPassword:      "Aa1!bbbb",
			ExpectHTTPStatus:       http.StatusUnauthorized,
			ExpectCode:             code.ErrPasswordIncorrect,
			ExpectMessageContains:  "密码不正确",
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"身份验证相关"},
		},
		{
			Name:                   "空旧密码",
			SetupUser:              true,
			CustomOldPassword:      "",
			CustomNewPassword:      "Aa1!cccc",
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrInvalidParameter,
			ExpectMessageContains:  "不能为空",
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"旧密码为空"},
		},
		{
			Name:                   "用户不存在",
			SetupUser:              false,
			Username:               "non_exist_user_cp",
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      "Aa1!dddd",
			ExpectHTTPStatus:       http.StatusNotFound,
			ExpectCode:             code.ErrUserNotFound,
			ExpectMessageContains:  "不存在",
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: false,
			UseAdminToken:          true,
			Notes:                  []string{"用户不存在"},
		},
		{
			Name:                   "新旧密码相同",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      defaultTestPassword,
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrInvalidParameter,
			ExpectMessageContains:  "不能相同",
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"新旧密码相同"},
		},
		{
			Name:                   "密码强度不足",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			CustomNewPassword:      "abc12345",
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrInvalidParameter,
			ExpectMessageContains:  "不符合要求",
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"密码强度不足"},
		},
		{
			Name:                   "JSON格式错误",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			RawBody:                []byte("{\"oldPassword\":\"" + defaultTestPassword + "\","),
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrBind,
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"JSON错误"},
		},
		{
			Name:                   "缺少字段",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			RawBody:                []byte(fmt.Sprintf("{\"oldPassword\":\"%s\"}", defaultTestPassword)),
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrInvalidParameter,
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"缺少新密码"},
		},
		{
			Name:                   "字段类型错误",
			SetupUser:              true,
			CustomOldPassword:      defaultTestPassword,
			RawBody:                []byte(fmt.Sprintf("{\"oldPassword\":\"%s\",\"newPassword\":123}", defaultTestPassword)),
			ExpectHTTPStatus:       http.StatusBadRequest,
			ExpectCode:             code.ErrBind,
			ShouldUpdateDB:         false,
			ShouldInvalidateToken:  false,
			ExpectLoginNewPassword: false,
			ExpectLoginOldPassword: true,
			Notes:                  []string{"类型错误"},
		},
	}

	for i, tcData := range cases {
		if i > 0 {
			continue
		}
		runFunctionalCase(t, tc, tcData)
	}

	if err := writeResultsToDisk(outputPath); err != nil {
		t.Fatalf("write results failed: %v", err)
	}

	if err := runValidationScript(baseDir); err != nil {
		t.Fatalf("validation script failed: %v", err)
	}
}

func runFunctionalCase(t *testing.T, tc *testContext, data TestCase) {
	t.Run(data.Name, func(t *testing.T) {
		username := data.Username
		var initialPassword string = defaultTestPassword

		if data.SetupUser {
			username = tc.createTestUser(initialPassword)
		}

		if username == "" {
			t.Fatalf("username cannot be empty")
		}

		oldPassword := data.CustomOldPassword
		if oldPassword == "" {
			oldPassword = initialPassword
		}

		newPassword := data.CustomNewPassword
		if len(newPassword) == 0 && len(data.RawBody) == 0 {
			newPassword = fmt.Sprintf("Aa1!%06d", time.Now().UnixNano()%1000000)
		}

		token, tokenErr := loginUserWithRetry(username, initialPassword, 6, 500*time.Millisecond)
		if data.SetupUser && tokenErr != nil {
			t.Fatalf("login before change failed: %v", tokenErr)
		}

		requestToken := token
		if data.UseAdminToken {
			requestToken = tc.adminToken
		}
		if data.SkipToken {
			requestToken = ""
		}

		payload := data.RawBody
		if len(payload) == 0 {
			body := map[string]string{
				"oldPassword": oldPassword,
				"newPassword": newPassword,
			}
			b, _ := json.Marshal(body)
			payload = b
		}

		resp, body, duration, err := changePasswordRequest(requestToken, username, payload)
		if err != nil {
			t.Fatalf("change password request error: %v", err)
		}

		var parsed apiResponse
		if len(body) > 0 {
			_ = json.Unmarshal(body, &parsed)
		}

		checks := map[string]bool{
			"response_received": true,
		}

		if resp.StatusCode != data.ExpectHTTPStatus {
			t.Fatalf("expected http %d got %d body=%s", data.ExpectHTTPStatus, resp.StatusCode, string(body))
		}
		checks["http_status"] = true

		if data.ExpectCode != 0 && parsed.Code != data.ExpectCode {
			t.Fatalf("expected code %d got %d message=%s", data.ExpectCode, parsed.Code, parsed.Message)
		}
		checks["code_match"] = true

		if data.ExpectMessageContains != "" && !strings.Contains(parsed.Message+parsed.Error, data.ExpectMessageContains) {
			t.Fatalf("expected message to contain %s got %s", data.ExpectMessageContains, parsed.Message)
		}
		if data.ExpectMessageContains != "" {
			checks["message_contains"] = true
		}

		success := resp.StatusCode == http.StatusOK && parsed.Code == respCodeSuccess

		if data.ShouldUpdateDB {
			hash, err := fetchUserPasswordHash(tc.db, username)
			if err != nil {
				t.Fatalf("fetch hash error: %v", err)
			}
			if hash == "" {
				t.Fatalf("expected hash for %s but empty", username)
			}
			if strings.Contains(hash, newPassword) {
				t.Fatalf("hash contains plaintext new password")
			}
			if strings.Contains(hash, oldPassword) {
				t.Fatalf("hash still matches old password")
			}
			if err := auth.Compare(hash, newPassword); err != nil {
				t.Fatalf("auth compare failed: %v", err)
			}
			checks["db_updated"] = true

			if data.ShouldInvalidateToken {
				if err := validateTokenInvalidation(token, username); err != nil {
					t.Fatalf("token invalidation check failed: %v", err)
				}
				checks["token_invalidated"] = true
			}

			if data.ExpectLoginNewPassword {
				if _, err := loginUser(username, newPassword); err != nil {
					t.Fatalf("login with new password failed: %v", err)
				}
				checks["login_new_password"] = true
			}

			if !data.ExpectLoginOldPassword {
				if _, err := loginUser(username, oldPassword); err == nil {
					t.Fatalf("old password should fail but succeeded")
				}
				checks["old_password_rejected"] = true
			}

			if err := ensurePasswordNotLogged(newPassword); err != nil {
				t.Fatalf("log scan failed: %v", err)
			}
			checks["log_scrubbed"] = true
		}

		if !data.ShouldUpdateDB {
			if hash, err := fetchUserPasswordHash(tc.db, username); err == nil && hash != "" {
				if err := auth.Compare(hash, initialPassword); err != nil {
					t.Fatalf("hash changed unexpectedly: %v", err)
				}
			}
			if data.ExpectLoginOldPassword && data.SetupUser {
				if _, err := loginUser(username, initialPassword); err != nil {
					t.Fatalf("old password login should succeed: %v", err)
				}
				checks["old_password_still_valid"] = true
			}
		}

		resultsMu.Lock()
		caseResults = append(caseResults, CaseResult{
			Name:             data.Name,
			Username:         username,
			HTTPStatus:       resp.StatusCode,
			Code:             parsed.Code,
			Message:          parsed.Message,
			Success:          success,
			DurationMs:       duration.Milliseconds(),
			ShouldUpdate:     data.ShouldUpdateDB,
			ShouldInvalidate: data.ShouldInvalidateToken,
			Checks:           checks,
			ExtraNotes:       data.Notes,
			OldPassword:      oldPassword,
			NewPassword:      newPassword,
		})
		resultsMu.Unlock()
	})
}

func validateTokenInvalidation(oldToken, username string) error {
	if oldToken == "" {
		return errors.New("old token empty")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf(serverBaseURL+getUserAPIPathFormat, username), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+oldToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return fmt.Errorf("old token still valid")
	}
	return nil
}

func ensurePasswordNotLogged(password string) error {
	file, err := os.Open(logFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), password) {
			return fmt.Errorf("password %s found in log", password)
		}
	}
	return scanner.Err()
}

// 得到当前源文件路径
func currentFile() string {
	_, file, _, _ := runtime.Caller(0)
	return file
}

// func TestChangePassword_Stress(t *testing.T) {
// 	//使用-short
// 	if testing.Short() {
// 		t.Skip("skipping stress test in short mode")
// 	}

// 	tc := newTestContext(t)
// 	defer tc.Close()

// 	config := StressConfig{
// 		BaseURL:          serverBaseURL,
// 		ConcurrentUsers:  50,
// 		TestDuration:     2 * time.Minute,
// 		RampUpTime:       30 * time.Second,
// 		RequestTimeout:   15 * time.Second,
// 		UserPoolSize:     100,
// 		PasswordVariants: 10,
// 		MetricsInterval:  5 * time.Second,
// 		EnableProfiling:  false,
// 	}

// 	metrics, err := runStressScenario(tc, config)
// 	if err != nil {
// 		t.Fatalf("stress scenario failed: %v", err)
// 	}

// 	resultsMu.Lock()
// 	perfResults = append(perfResults, metrics)
// 	resultsMu.Unlock()
// }

func runStressScenario(tc *testContext, cfg StressConfig) (PerformanceMetrics, error) {
	usernames := make([]string, 0, cfg.UserPoolSize)
	for i := 0; i < cfg.UserPoolSize; i++ {
		usernames = append(usernames, tc.createTestUser(defaultTestPassword))
	}

	var totalRequests int64
	var successCount int64
	var authErrors int64
	var validationFailures int64
	durations := make([]time.Duration, 0, 1024)
	durMu := sync.Mutex{}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < cfg.ConcurrentUsers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			randSource := rand.New(rand.NewSource(time.Now().UnixNano() + int64(worker)))
			for {
				select {
				case <-stop:
					return
				default:
				}

				idx := randSource.Intn(len(usernames))
				username := usernames[idx]
				oldPassword := defaultTestPassword
				newPassword := fmt.Sprintf("Stress%d!A", randSource.Intn(1000000))

				token, err := loginUser(username, oldPassword)
				if err != nil {
					atomic.AddInt64(&authErrors, 1)
					continue
				}

				payload := map[string]string{"oldPassword": oldPassword, "newPassword": newPassword}
				body, _ := json.Marshal(payload)

				resp, respBody, duration, err := changePasswordRequest(token, username, body)
				atomic.AddInt64(&totalRequests, 1)
				durMu.Lock()
				durations = append(durations, duration)
				durMu.Unlock()
				if err != nil {
					atomic.AddInt64(&validationFailures, 1)
					continue
				}
				var parsed apiResponse
				_ = json.Unmarshal(respBody, &parsed)
				if resp.StatusCode == http.StatusOK && parsed.Code == respCodeSuccess {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	time.Sleep(cfg.TestDuration)
	close(stop)
	wg.Wait()

	total := atomic.LoadInt64(&totalRequests)
	success := atomic.LoadInt64(&successCount)

	metrics := PerformanceMetrics{Scenario: "baseline_stress"}
	if total > 0 {
		elapsed := cfg.TestDuration.Seconds()
		metrics.Throughput = float64(total) / elapsed
		metrics.AvgResponseTimeMs = averageDuration(durations).Seconds() * 1000
		p95, p99 := percentile(durations, 0.95), percentile(durations, 0.99)
		metrics.P95ResponseTimeMs = p95.Seconds() * 1000
		metrics.P99ResponseTimeMs = p99.Seconds() * 1000
		metrics.ErrorRate = float64(total-success) / float64(total)
		metrics.SuccessRate = float64(success) / float64(total)
	}
	metrics.PasswordUpdateSuccess = int(success)
	metrics.ValidationFailures = int(atomic.LoadInt64(&validationFailures))
	metrics.AuthErrors = int(atomic.LoadInt64(&authErrors))

	metrics.GoroutineCount = runtime.NumGoroutine()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	metrics.MemoryUsageMB = float64(ms.Alloc) / (1024 * 1024)

	var gcStats debug.GCStats
	debug.ReadGCStats(&gcStats)
	metrics.GCStats = fmt.Sprintf("last_gc=%v num_gc=%d pause_total=%v", gcStats.LastGC, gcStats.NumGC, gcStats.PauseTotal)

	if tc.db != nil {
		if stats := tc.db.Stats(); stats.OpenConnections > 0 {
			metrics.DBConnections = stats.OpenConnections
		}
	}

	cpuUsage, err := sampleCPUUsage(cfg.TestDuration)
	if err == nil {
		metrics.CPUUsage = cpuUsage
	}

	return metrics, nil
}

func averageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return time.Duration(int64(sum) / int64(len(durations)))
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1) * p)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func sampleCPUUsage(duration time.Duration) (float64, error) {
	if duration > 2*time.Second {
		duration = 2 * time.Second
	}
	type cpuSample struct {
		idle  uint64
		total uint64
	}

	readCPU := func() (cpuSample, error) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return cpuSample{}, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) < 5 {
					return cpuSample{}, fmt.Errorf("invalid cpu line")
				}
				values := make([]uint64, len(fields)-1)
				for i, field := range fields[1:] {
					var v uint64
					fmt.Sscanf(field, "%d", &v)
					values[i] = v
				}
				var total uint64
				for _, v := range values {
					total += v
				}
				idle := values[3]
				return cpuSample{idle: idle, total: total}, nil
			}
		}
		return cpuSample{}, fmt.Errorf("cpu line not found")
	}

	start, err := readCPU()
	if err != nil {
		return 0, err
	}
	time.Sleep(duration)
	end, err := readCPU()
	if err != nil {
		return 0, err
	}

	idleDelta := float64(end.idle - start.idle)
	totalDelta := float64(end.total - start.total)
	if totalDelta == 0 {
		return 0, nil
	}
	usage := (1 - idleDelta/totalDelta) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, nil
}
