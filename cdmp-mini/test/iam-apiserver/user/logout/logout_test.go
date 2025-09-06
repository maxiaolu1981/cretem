package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ==================== 全局配置 ====================
var (
	// 接口配置（根据实际业务修改）
	apiBaseURL   = "http://127.0.0.1:8080"                                                // 基础URL，用于拼接登录/登出接口
	apiLoginURL  = apiBaseURL + "/login"                                                  // 登录接口地址
	apiLogoutURL = apiBaseURL + "/logout"                                                 // 登出接口地址
	expiredToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTcwMDAwMDB9.xxxxxx" // 已过期令牌
	invalidToken = "invalid_token_123456"                                                 // 格式无效令牌
	timeout      = 10 * time.Second                                                       // 请求超时时间

	// 颜色配置
	ansiReset  = "\033[0m"    // 重置颜色
	ansiGreen  = "\033[32;1m" // 绿色加粗（成功）
	ansiRed    = "\033[31;1m" // 红色加粗（失败）
	ansiBlue   = "\033[34m"   // 蓝色（信息）
	ansiYellow = "\033[33;1m" // 黄色加粗（用例标题）
	ansiPurple = "\033[35m"   // 紫色（测试阶段）

	// 测试统计
	total  int
	passed int
	failed int
)

// ==================== 响应体结构 ====================
// 登出接口的标准响应格式
type LogoutResponse struct {
	Code    int         `json:"code"`    // 业务状态码（0=成功，非0=失败）
	Message string      `json:"message"` // 人类可读提示
	Data    interface{} `json:"data"`    // 可选数据
}

// 登录请求体（根据实际登录接口参数调整）
type LoginRequest struct {
	Username string `json:"username"` // 登录用户名
	Password string `json:"password"` // 登录密码
}

// 登录响应体（根据实际登录接口返回调整）
type LoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"` // 登录返回的有效令牌
	} `json:"data"`
}

// ==================== 工具函数 ====================
// 动态登录获取有效令牌
func getValidToken() (string, error) {
	// 从环境变量获取登录凭证（避免硬编码敏感信息）
	// username := os.Getenv("TEST_USERNAME")
	// password := os.Getenv("TEST_PASSWORD")
	username := "admin"
	password := "Admin@2021"
	if username == "" || password == "" {
		return "", fmt.Errorf("请先设置环境变量 TEST_USERNAME 和 TEST_PASSWORD")
	}

	// 构造登录请求
	loginReq := LoginRequest{
		Username: username,
		Password: password,
	}
	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("构造登录请求失败: %w", err)
	}

	// 发送登录请求
	req, err := http.NewRequest(http.MethodPost, apiLoginURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析登录响应
	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %w", err)
	}

	// 验证登录成功且令牌有效
	if resp.StatusCode != http.StatusOK || loginResp.Code != 0 || loginResp.Data.Token == "" {
		return "", fmt.Errorf("登录失败: 状态码=%d, 业务码=%d, 消息=%s",
			resp.StatusCode, loginResp.Code, loginResp.Message)
	}

	fmt.Printf("%s✅ 登录成功，获取有效令牌（前10位）: %s%s\n",
		ansiGreen, loginResp.Data.Token[:10], ansiReset)
	return loginResp.Data.Token, nil
}

// 发送登出请求
func sendLogoutRequest(method, token string) (*http.Response, LogoutResponse, error) {
	var emptyResp LogoutResponse

	// 创建请求
	req, err := http.NewRequest(method, apiLogoutURL, nil)
	if err != nil {
		return nil, emptyResp, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	// 发送请求
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return resp, emptyResp, fmt.Errorf("发送请求失败: %w", err)
	}

	// 读取并解析响应体
	defer resp.Body.Close()
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	respBody := strings.TrimSpace(string(body[:n]))

	// 解析JSON（处理204 No Content特殊场景）
	var logoutResp LogoutResponse
	if err := json.Unmarshal([]byte(respBody), &logoutResp); err != nil {
		if resp.StatusCode == http.StatusNoContent && respBody == "" {
			return resp, logoutResp, nil
		}
		return resp, emptyResp, fmt.Errorf("响应格式不合规: %w，原始响应: %s", err, respBody)
	}

	return resp, logoutResp, nil
}

// ==================== 测试用例执行函数 ====================
func runTestCase(
	t *testing.T,
	testName string,
	httpMethod string,
	token string,
	expectedStatus int,
	expectedCode int,
	expectedMsg string,
	allowEmptyBody bool,
) {
	total++
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	fmt.Printf("%s接口地址: %s%s\n", ansiBlue, apiLogoutURL, ansiReset)
	fmt.Printf("%s请求方法: %s, 预期状态码: %d%s\n", ansiBlue, httpMethod, expectedStatus, ansiReset)
	fmt.Printf("%s预期业务码: %d, 预期消息: %s%s\n", ansiBlue, expectedCode, expectedMsg, ansiReset)

	// 发送请求
	resp, respBody, err := sendLogoutRequest(httpMethod, token)
	if err != nil {
		fmt.Printf("%s❌ 请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」终止: %v", testName, err)
	}

	// 校验HTTP状态码
	statusPassed := resp.StatusCode == expectedStatus
	if !statusPassed {
		fmt.Printf("%s❌ 状态码不符: 实际 %d, 预期 %d%s\n", ansiRed, resp.StatusCode, expectedStatus, ansiReset)
	} else {
		fmt.Printf("%s✅ 状态码正确: %d%s\n", ansiGreen, resp.StatusCode, ansiReset)
	}

	// 校验响应体
	bodyPassed := true
	if !(allowEmptyBody && resp.StatusCode == http.StatusNoContent) {
		// 校验业务码
		if respBody.Code != expectedCode {
			fmt.Printf("%s❌ 业务码不符: 实际 %d, 预期 %d%s\n", ansiRed, respBody.Code, expectedCode, ansiReset)
			bodyPassed = false
		}
		// 校验提示消息
		if !strings.Contains(respBody.Message, expectedMsg) {
			fmt.Printf("%s❌ 消息不符: 实际「%s」, 预期包含「%s」%s\n", ansiRed, respBody.Message, expectedMsg, ansiReset)
			bodyPassed = false
		}
		// 打印响应体
		respJson, _ := json.MarshalIndent(respBody, "", "  ")
		fmt.Printf("%s响应内容: %s%s\n", ansiBlue, string(respJson), ansiReset)
	} else {
		fmt.Printf("%s响应内容: 符合预期（204 No Content）%s\n", ansiBlue, ansiReset)
	}

	// 统计结果
	casePassed := statusPassed && bodyPassed
	if casePassed {
		fmt.Printf("%s✅ 用例通过: 符合 RESTful 规范%s\n", ansiGreen, ansiReset)
		passed++
	} else {
		fmt.Printf("%s❌ 用例失败: 不符合 RESTful 规范%s\n", ansiRed, ansiReset)
		failed++
		t.Errorf("用例「%s」失败: 状态码或响应内容不符", testName)
	}
	fmt.Println("----------------------------------------")
}

// ==================== 测试用例 ====================
// 1. 正常登出（有效令牌）
func caseNormalLogout(t *testing.T) {
	// 动态获取有效令牌
	validToken, err := getValidToken()
	if err != nil {
		t.Fatalf("获取有效令牌失败: %v", err)
	}

	runTestCase(
		t,
		"正常登出（有效令牌）",
		http.MethodDelete,
		validToken,
		http.StatusOK,
		0,
		"登出成功",
		false,
	)
}

// 2. 无令牌登出
func caseNoTokenLogout(t *testing.T) {
	runTestCase(
		t,
		"无令牌登出",
		http.MethodDelete,
		"",
		http.StatusUnauthorized,
		100205,
		"请先登录",
		false,
	)
}

// 3. 无效令牌登出（格式错误）
func caseInvalidTokenLogout(t *testing.T) {
	runTestCase(
		t,
		"无效令牌登出（格式错误）",
		http.MethodDelete,
		invalidToken,
		http.StatusBadRequest,
		100208,
		"令牌格式错误",
		false,
	)
}

// 4. 过期令牌登出
func caseExpiredTokenLogout(t *testing.T) {
	runTestCase(
		t,
		"过期令牌登出",
		http.MethodDelete,
		expiredToken,
		http.StatusUnauthorized,
		10003,
		"令牌已过期",
		false,
	)
}

// 5. 重复登出（幂等性测试）
func caseDuplicateLogout(t *testing.T) {
	// 动态获取新令牌
	validToken, err := getValidToken()
	if err != nil {
		t.Fatalf("获取有效令牌失败: %v", err)
	}

	// 第一次登出
	_, _, _ = sendLogoutRequest(http.MethodDelete, validToken)

	// 第二次登出（同一令牌）
	runTestCase(
		t,
		"重复登出（幂等性）",
		http.MethodDelete,
		validToken,
		http.StatusOK,
		0,
		"已登出",
		false,
	)
}

// 6. 错误HTTP方法
func caseWrongMethodLogout(t *testing.T) {
	// 动态获取有效令牌
	validToken, err := getValidToken()
	if err != nil {
		t.Fatalf("获取有效令牌失败: %v", err)
	}

	runTestCase(
		t,
		"错误HTTP方法（GET）",
		http.MethodGet,
		validToken,
		http.StatusMethodNotAllowed,
		10004,
		"不支持GET方法",
		false,
	)
}

// 7. 登出后令牌失效验证
func caseTokenInvalidAfterLogout(t *testing.T) {
	// 动态获取有效令牌
	validToken, err := getValidToken()
	if err != nil {
		t.Fatalf("获取有效令牌失败: %v", err)
	}

	// 执行登出
	_, _, _ = sendLogoutRequest(http.MethodDelete, validToken)

	// 验证令牌失效（调用用户信息接口）
	testUserInfoURL := apiBaseURL + "/users/me"
	req, _ := http.NewRequest(http.MethodGet, testUserInfoURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", validToken))
	client := &http.Client{Timeout: timeout}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	// 解析响应并验证
	var userResp LogoutResponse
	json.NewDecoder(resp.Body).Decode(&userResp)
	runTestCase(
		t,
		"登出后令牌失效验证",
		http.MethodGet,
		validToken,
		http.StatusUnauthorized,
		10003,
		"令牌已失效",
		false,
	)
}

// ==================== 测试入口 ====================
func TestUserLogout_Complete(t *testing.T) {
	// 前置检查
	if apiBaseURL == "" {
		fmt.Printf("%s❌ 请先配置 apiBaseURL！%s\n", ansiRed, ansiReset)
		t.Fatal("配置不完整，测试终止")
	}

	// 打印测试开始信息
	fmt.Println("================================================================================")
	fmt.Printf("%s开始执行登出接口全量测试（%s）%s\n", ansiPurple, time.Now().Format("2006-01-02 15:04:05"), ansiReset)
	fmt.Printf("%sRESTful 规范校验: 方法/状态码/响应格式/幂等性%s\n", ansiPurple, ansiReset)
	fmt.Println("================================================================================")

	// 初始化统计
	total, passed, failed = 0, 0, 0

	// 执行测试用例
	t.Run("用例1_正常登出", caseNormalLogout)
	t.Run("用例2_无令牌登出", caseNoTokenLogout)
	t.Run("用例3_无效令牌登出", caseInvalidTokenLogout)
	t.Run("用例4_过期令牌登出", caseExpiredTokenLogout)
	t.Run("用例5_重复登出（幂等性）", caseDuplicateLogout)
	t.Run("用例6_错误HTTP方法", caseWrongMethodLogout)
	t.Run("用例7_登出后令牌失效验证", caseTokenInvalidAfterLogout)

	// 打印测试总结
	fmt.Println("\n================================================================================")
	fmt.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d\n", total, passed, failed)
	fmt.Println("================================================================================")

	// 结果提示
	if failed > 0 {
		fmt.Printf("%s❌ 测试未通过，存在 %d 个不符合 RESTful 规范的场景%s\n", ansiRed, failed, ansiReset)
		t.Fatalf("共有 %d 个用例失败", failed)
	} else {
		fmt.Printf("%s🎉 所有登出接口测试用例全部通过，完全符合 RESTful 规范！%s\n", ansiGreen, ansiReset)
	}
}
