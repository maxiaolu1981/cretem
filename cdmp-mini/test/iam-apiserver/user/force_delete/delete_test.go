package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// ==================== 全局配置（新增登录配置，删除硬编码Token） ====================
var (
	apiBaseURL = "http://127.0.0.1:8080/v1/users" // 硬删除接口基础URL
	loginURL   = "http://127.0.0.1:8080/login"    // 登录接口URL（需根据实际项目调整）
	timeout    = 10 * time.Second                 // 超时时间

	// 测试账号（需替换为实际测试环境的管理员账号，删除接口需管理员权限）
	adminUsername = "admin"      // 管理员用户名
	adminPassword = "Admin@2021" // 管理员密码（替换为真实密码）

	// 动态生成的Token（测试前自动获取，替代原硬编码）
	token        string
	validUser    = "gettest-user104"                                   // 确保存在的用户（204测试用）
	nonExistUser = "non_exist_" + fmt.Sprintf("%d", time.Now().Unix()) // 不存在的用户（404测试用）
	invalidUser  = "invalid@user"                                      // 格式无效的用户（400测试用）

	// 原生ANSI颜色码
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32;1m"
	ansiRed    = "\033[31;1m"
	ansiBlue   = "\033[34m"
	ansiPurple = "\033[35m"
	ansiYellow = "\033[33;1m"

	// 测试统计
	total  int
	passed int
	failed int
)

// ==================== 新增：登录相关结构体（与实际登录接口响应匹配） ====================
// LoginRequest 登录请求体（字段名需与登录接口参数一致）
type LoginRequest struct {
	Username string `json:"username"` // 与登录接口的“用户名”参数名匹配（区分大小写）
	Password string `json:"password"` // 与登录接口的“密码”参数名匹配
}

// LoginResponse 登录响应体（与实际登录接口返回格式完全匹配）
// 参考你之前的登录响应：{"code":0,"data":{"token":"xxx","expire":"2025-09-08T16:11:07+08:00"},"message":""}
type LoginResponse struct {
	Code    int    `json:"code"`    // 业务状态码（0=成功，与你的接口一致）
	Message string `json:"message"` // 提示信息（可为空）
	Data    struct {
		Token  string `json:"token"`  // Token字段（与你的响应字段“token”匹配）
		Expire string `json:"expire"` // 有效期字段（与你的响应字段“expire”匹配）
	} `json:"data"` // 数据体（与你的接口响应结构一致）
}

// ==================== 工具函数（新增Token自动获取，完善原有请求函数） ====================
// sendPostRequest 通用POST请求（用于发送登录请求）
func sendPostRequest(url string, reqBody []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("创建POST请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("发送POST请求失败: %w", err)
	}

	defer resp.Body.Close()
	body := make([]byte, 1024*1024)
	n, err := resp.Body.Read(body)
	if err != nil && err.Error() != "EOF" {
		return resp, nil, fmt.Errorf("读取POST响应失败: %w", err)
	}
	return resp, body[:n], nil
}

// getToken 自动调用登录接口，获取管理员Token
func getToken(username, password string) (string, error) {
	fmt.Printf("%s[INFO] 正在为管理员「%s」获取Token...%s\n", ansiBlue, username, ansiReset)

	// 1. 构建登录请求体
	loginReq := LoginRequest{Username: username, Password: password}
	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("登录请求JSON序列化失败: %w", err)
	}

	// 2. 发送登录请求
	resp, respBody, err := sendPostRequest(loginURL, reqBody)
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %w", err)
	}

	// 3. 校验登录请求的HTTP状态码（登录接口通常返回200 OK）
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"登录HTTP状态码错误: 预期200，实际%d，响应内容: %s",
			resp.StatusCode, string(respBody),
		)
	}

	// 4. 解析登录响应
	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", fmt.Errorf(
			"登录响应解析失败: %w，响应内容: %s",
			err, string(respBody),
		)
	}

	// 5. 校验业务逻辑（code=0为成功，与你的接口一致）
	if loginResp.Code != 0 {
		return "", fmt.Errorf(
			"登录业务失败: 业务码%d，提示: %s，响应内容: %s",
			loginResp.Code, loginResp.Message, string(respBody),
		)
	}

	// 6. 校验Token是否为空
	if loginResp.Data.Token == "" {
		return "", fmt.Errorf(
			"登录响应未包含Token，响应内容: %s",
			string(respBody),
		)
	}

	// 7. 返回有效Token（附带有效期提示）
	fmt.Printf(
		"%s[INFO] 管理员Token获取成功，有效期至: %s%s\n",
		ansiGreen, loginResp.Data.Expire, ansiReset,
	)
	return loginResp.Data.Token, nil
}

// sendDeleteRequest 发送DELETE请求（原有逻辑不变，Token来源改为自动获取）
func sendDeleteRequest(url string, headers map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建DELETE请求失败: %w", err)
	}

	// 设置默认头
	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头（如Authorization）
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("发送DELETE请求失败: %w", err)
	}

	// 读取响应体
	defer resp.Body.Close()
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	return resp, string(body[:n]), nil
}

// ==================== 测试用例执行函数（原有逻辑不变，Token自动传入） ====================
func runTestCase(t *testing.T, testName, url string, expectedCode int, headers map[string]string) {
	total++
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	fmt.Printf("%s描述: 预期状态码 %d，请求地址: %s%s\n", ansiBlue, expectedCode, url, ansiReset)

	// 发送请求
	resp, body, err := sendDeleteRequest(url, headers)
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	// 验证状态码
	actualCode := resp.StatusCode
	casePassed := actualCode == expectedCode

	// 输出结果
	if casePassed {
		fmt.Printf("%s✅ 状态码正确: 预期 %d, 实际 %d%s\n", ansiGreen, expectedCode, actualCode, ansiReset)
	} else {
		fmt.Printf("%s❌ 状态码错误: 预期 %d, 实际 %d%s\n", ansiRed, expectedCode, actualCode, ansiReset)
	}

	// 输出响应体
	fmt.Printf("%s响应体: %s%s\n", ansiBlue, body, ansiReset)

	// 统计结果
	if casePassed {
		fmt.Printf("%s----------------------------------------%s\n", ansiGreen, ansiReset)
		fmt.Printf("%s✅ 用例执行通过 %s\n", ansiGreen, ansiReset)
		passed++
	} else {
		fmt.Printf("%s----------------------------------------%s\n", ansiRed, ansiReset)
		fmt.Printf("%s❌ 用例执行失败 %s\n", ansiRed, ansiReset)
		failed++
		t.Errorf("用例「%s」失败: 状态码不匹配", testName)
	}
}

// ==================== 测试用例实现（原有逻辑不变，Token从自动获取的变量读取） ====================
// 1. 400参数错误（用户名为空）
func case400EmptyUser(t *testing.T) {
	url := fmt.Sprintf("%s//force", apiBaseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token), // 自动获取的Token
	}
	runTestCase(t, "400参数错误（用户名为空）", url, http.StatusBadRequest, headers)
}

// 2. 400参数错误（用户名含特殊字符@）
func case400InvalidChar(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, invalidUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token), // 自动获取的Token
	}
	runTestCase(t, "400参数错误（用户名含@）", url, http.StatusBadRequest, headers)
}

// 3. 401未授权（无令牌）
func case401NoToken(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{} // 无令牌（原有逻辑不变）
	runTestCase(t, "401未授权（无令牌）", url, http.StatusUnauthorized, headers)
}

// 4. 401未授权（无效令牌）
func case401InvalidToken(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{
		"Authorization": "Bearer invalid_token_xxx", // 无效Token（原有逻辑不变）
	}
	runTestCase(t, "401未授权（无效令牌）", url, http.StatusUnauthorized, headers)
}

// 5. 404用户不存在
func case404UserNotFound(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token), // 自动获取的Token
	}
	runTestCase(t, "404用户不存在", url, http.StatusNotFound, headers)
}

// 6. 204删除成功（有效用户）
func case204DeleteSuccess(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, validUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token), // 自动获取的Token
	}
	runTestCase(t, "204删除成功（有效用户）", url, http.StatusNoContent, headers)
}

// 7. 500服务器内部错误（需后端配合）
func case500InternalError(t *testing.T) {
	url := fmt.Sprintf("%s/trigger_500/force", apiBaseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token), // 自动获取的Token
	}
	runTestCase(t, "500服务器内部错误", url, http.StatusInternalServerError, headers)
}

// ==================== 测试入口（新增Token自动初始化，优先于所有用例执行） ====================
func TestDeleteUser_AllCases(t *testing.T) {
	// 第一步：自动获取管理员Token（所有删除用例需管理员权限，优先执行）
	var err error
	token, err = getToken(adminUsername, adminPassword)
	if err != nil {
		fmt.Printf("%s❌ 管理员Token获取失败: %v%s\n", ansiRed, err, ansiReset)
		t.Fatal("Token获取失败，测试终止（请检查登录接口、账号密码是否正确）")
	}

	// 第二步：验证核心配置（Token和有效用户）
	if token == "" || validUser == "" {
		fmt.Printf("%s❌ Token为空或validUser未配置，测试终止%s\n", ansiRed, ansiReset)
		t.Fatal("核心配置不完整")
	}
	fmt.Printf("%s[INFO] Token初始化完成，开始执行硬删除接口测试...%s\n", ansiGreen, ansiReset)

	// 第三步：执行原有测试流程
	fmt.Println("================================================================================")
	fmt.Printf("%s开始执行用户硬删除接口测试（%s）%s\n", ansiBlue, time.Now().Format(time.RFC3339), ansiReset)
	fmt.Println("================================================================================")

	// 初始化统计
	total, passed, failed = 0, 0, 0

	// 执行测试用例（原有顺序不变）
	t.Run("用例1_400空用户名", func(t *testing.T) { case400EmptyUser(t) })
	t.Run("用例2_400特殊字符", func(t *testing.T) { case400InvalidChar(t) })
	t.Run("用例3_401无令牌", func(t *testing.T) { case401NoToken(t) })
	t.Run("用例4_401无效令牌", func(t *testing.T) { case401InvalidToken(t) })
	t.Run("用例5_404不存在用户", func(t *testing.T) { case404UserNotFound(t) })
	t.Run("用例6_204删除成功", func(t *testing.T) { case204DeleteSuccess(t) })
	// 如需测试500，取消下面一行注释（需后端配合构造500场景）
	// t.Run("用例7_500服务器错误", func(t *testing.T) { case500InternalError(t) })

	// 测试总结
	fmt.Println("\n================================================================================")
	fmt.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d", total, passed, failed)
	fmt.Println("================================================================================")

	// 输出结果
	if failed > 0 {
		fmt.Printf("%s❌ 存在%d个失败用例，请检查：%s\n", ansiRed, failed, ansiReset)
		fmt.Printf("%s1. 配置区的validUser（%s）是否真的存在？%s\n", ansiRed, validUser, ansiReset)
		fmt.Printf("%s2. 后端硬删除接口是否正确返回对应状态码？%s\n", ansiRed, ansiReset)
		fmt.Printf("%s3. 管理员账号（%s）是否有硬删除权限？%s\n", ansiRed, adminUsername, ansiReset)
		t.Fatalf("共有 %d 个用例失败", failed)
	} else {
		fmt.Printf("%s🎉 所有测试用例全部通过!%s\n", ansiGreen, ansiReset)
	}
}
