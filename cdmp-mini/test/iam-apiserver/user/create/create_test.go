package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

// ==================== 全局配置 ====================
var (
	baseURL      = "http://localhost:8080/v1/users"
	adminToken   = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzIxNTE0OSwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTcxMjg3NDksInN1YiI6ImFkbWluIn0.eHtg8U81RTlKPfgH5Y4hOkfMcdkgltO4POwcGzeDcuA"
	userToken    = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzIxNTE5OSwiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDQiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdfaWF0IjoxNzU3MTI4Nzk5LCJzdWIiOiJnZXR0ZXN0LXVzZXIxMDQifQ.yy5HrwqH82Lkf-lFKP-ihwIN7VJF0ukQNzQLj3mtEzc"
	tempDir      = "./temp_json"
	testUsername = fmt.Sprintf("testuser%d", time.Now().Unix())
	testEmail    = fmt.Sprintf("test%d@example.com", time.Now().Unix())

	// 颜色定义
	colorPass  = color.New(color.FgGreen).Add(color.Bold)
	colorFail  = color.New(color.FgRed).Add(color.Bold)
	colorInfo  = color.New(color.FgBlue)
	colorCase  = color.New(color.FgYellow).Add(color.Bold)
	colorBlue  = color.New(color.FgBlue).Add(color.Bold)
	colorReset = color.New(color.Reset)

	// 测试统计
	total  int
	passed int
	failed int
)

// ==================== 数据结构 ====================
type UserRequest struct {
	Metadata struct {
		Name       string                 `json:"name"`
		InstanceID string                 `json:"instanceID"`
		Extend     map[string]interface{} `json:"extend"`
	} `json:"metadata"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Nickname  string `json:"nickname"`
	Phone     string `json:"phone,omitempty"`
	Status    int    `json:"status"`
	LoginedAt string `json:"loginedAt"`
}

type Response struct {
	Code    int         `json:"code,omitempty"`
	Message string      `json:"message,omitempty"`
	Msg     string      `json:"msg,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ==================== 工具函数 ====================
func initTempDir() error {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return fmt.Errorf("创建临时目录失败: %w", err)
		}
		colorInfo.Printf("[INFO] 临时目录创建成功: %s\n", tempDir)
	}
	return nil
}

func generateInstanceID() string {
	return fmt.Sprintf("usr%d", time.Now().Unix())
}

func cleanupTemp() {
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		if err := os.RemoveAll(tempDir); err != nil {
			colorInfo.Printf("[INFO] 临时文件清理失败: %v\n", err)
			return
		}
		colorInfo.Println("[INFO] 临时文件已清理")
	}
}

func saveJSONToTemp(filename string, data interface{}) (string, error) {
	if err := initTempDir(); err != nil {
		return "", err
	}

	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return "", fmt.Errorf("JSON 序列化失败: %w", err)
	}

	filePath := filepath.Join(tempDir, filename)
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}

	return filePath, nil
}

func sendPostRequest(url, token string, requestBody []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("发送请求失败: %w", err)
	}

	defer resp.Body.Close()
	respBody := make([]byte, 1024*1024)
	n, err := resp.Body.Read(respBody)
	if err != nil && err.Error() != "EOF" {
		return resp, nil, fmt.Errorf("读取响应失败: %w", err)
	}

	return resp, respBody[:n], nil
}

func parseResponse(respBody []byte) (int, string, string) {
	var resp Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return 0, "", fmt.Sprintf("JSON 解析失败: %v", err)
	}

	message := resp.Message
	if message == "" {
		message = resp.Msg
	}

	return resp.Code, message, string(respBody)
}

// ==================== 测试执行函数 ====================
func runTestCase(t *testing.T, testName, description string, req UserRequest, token string, expectedHTTPStatus int, expectedMsg string) {
	total++
	colorCase.Printf("\n用例 %d: %s\n", total, testName)
	fmt.Println("----------------------------------------")
	colorInfo.Printf("描述: %s\n", description)
	colorInfo.Println("请求体JSON内容（语法校验后）:")

	jsonFile, err := saveJSONToTemp(fmt.Sprintf("test%d.json", total), req)
	if err != nil {
		colorFail.Printf("❌ %v\n", err)
		t.Fatalf("用例「%s」准备失败: %v", testName, err)
	}
	defer os.Remove(jsonFile)

	jsonContent, _ := os.ReadFile(jsonFile)
	fmt.Println(string(jsonContent))

	resp, respBody, err := sendPostRequest(baseURL, token, jsonContent)
	if err != nil {
		colorFail.Printf("❌ 发送请求失败: %v\n", err)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

	// 验证HTTP状态码
	if actualHTTPStatus == expectedHTTPStatus {
		colorPass.Printf("✅ 状态码正确: %d\n", actualHTTPStatus)
	} else {
		colorFail.Printf("❌ 状态码错误: 预期 %d, 实际 %d\n", expectedHTTPStatus, actualHTTPStatus)
	}

	// 验证业务码和消息
	casePassed := true
	if actualHTTPStatus != expectedHTTPStatus {
		casePassed = false
	}
	if !strings.Contains(respMsg, expectedMsg) {
		colorFail.Printf("❌ 消息错误: 预期包含「%s」, 实际「%s」\n", expectedMsg, respMsg)
		casePassed = false
	} else {
		colorPass.Printf("✅ 消息正确: %s\n", respMsg)
	}

	if casePassed {
		colorPass.Println("----------------------------------------")
		colorPass.Printf("用例执行通过 ✅\n")
		passed++
	} else {
		colorFail.Println("----------------------------------------")
		colorFail.Printf("用例执行失败 ❌\n")
		failed++
	}
}

// ==================== 10个完整测试用例 ====================

// 1. 使用正确参数创建用户
func TestCreateUser_ValidParams(t *testing.T) {
	req := UserRequest{
		Email:     testEmail,
		Password:  "ValidPass123!",
		Nickname:  "TestUserNickname",
		Phone:     "13800138000",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = testUsername
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"使用正确参数创建用户",
		"JSON无注释+含必填nickname，应返回201",
		req,
		adminToken,
		http.StatusCreated,
		"用户创建成功",
	)
}

// 2. 创建已存在的用户
func TestCreateUser_DuplicateUsername(t *testing.T) {
	req := UserRequest{
		Email:     fmt.Sprintf("another%s", testEmail),
		Password:  "ValidPass123!",
		Nickname:  "DuplicateNickname",
		Phone:     "13900139000",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = testUsername // 使用与用例1相同的用户名
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"创建已存在的用户",
		"用户名重复，应返回409",
		req,
		adminToken,
		http.StatusConflict,
		"用户已经存在",
	)
}

// 3. 缺少必填字段（metadata.name）
func TestCreateUser_MissingRequiredField(t *testing.T) {
	req := UserRequest{
		Email:     "missingusername@example.com",
		Password:  "ValidPass123!",
		Nickname:  "MissingNameNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	// 故意不设置metadata.name（必填字段）
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"缺少必填字段（metadata.name）",
		"用户名缺失，应返回422",
		req,
		adminToken,
		http.StatusUnprocessableEntity,
		"名称部分不能为空",
	)
}

// 4. 用户名不合法（含@）
func TestCreateUser_InvalidUsername(t *testing.T) {
	req := UserRequest{
		Email:     "invalidusername@example.com",
		Password:  "ValidPass123!",
		Nickname:  "InvalidNameNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = "invalid@username" // 包含非法字符@
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"用户名不合法（含@）",
		"用户名含特殊字符，应返回422",
		req,
		adminToken,
		http.StatusUnprocessableEntity,
		"名称部分必须由字母、数字、'-'、'_'或'.'组成",
	)
}

// 5. 密码不符合规则（弱密码123）
func TestCreateUser_WeakPassword(t *testing.T) {
	req := UserRequest{
		Email:     "weakpass@example.com",
		Password:  "123", // 弱密码
		Nickname:  "WeakPassNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = fmt.Sprintf("weakpassuser%d", time.Now().Unix())
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"密码不符合规则（弱密码123）",
		"密码不满足规则，应返回422",
		req,
		adminToken,
		http.StatusUnprocessableEntity,
		"密码设定不符合规则",
	)
}

// 6. 未提供Authorization头
func TestCreateUser_NoAuthHeader(t *testing.T) {
	req := UserRequest{
		Email:     "noauth@example.com",
		Password:  "ValidPass123!",
		Nickname:  "NoAuthNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = fmt.Sprintf("noauthuser%d", time.Now().Unix())
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"未提供Authorization头",
		"无认证令牌，应返回401",
		req,
		"", // 不传递token
		http.StatusUnauthorized,
		"缺少 Authorization 头",
	)
}

// 7. 使用无效token
func TestCreateUser_InvalidToken(t *testing.T) {
	req := UserRequest{
		Email:     "invalidtoken@example.com",
		Password:  "ValidPass123!",
		Nickname:  "InvalidTokenNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = fmt.Sprintf("invalidtokenuser%d", time.Now().Unix())
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"使用无效token",
		"令牌格式错误，应返回401",
		req,
		"invalid-token", // 无效token
		http.StatusUnauthorized,
		"token contains an invalid number of segments",
	)
}

// 8. 权限不足（普通用户创建用户）
func TestCreateUser_Forbidden(t *testing.T) {
	req := UserRequest{
		Email:     "forbidden@example.com",
		Password:  "ValidPass123!",
		Nickname:  "ForbiddenNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = fmt.Sprintf("forbiddenuser%d", time.Now().Unix())
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"权限不足（普通用户创建用户）",
		"普通用户无创建权限，应返回403",
		req,
		userToken, // 使用普通用户token
		http.StatusForbidden,
		"权限不足",
	)
}

// 9. 请求格式错误（非JSON）
func TestCreateUser_NonJSONBody(t *testing.T) {
	testName := "请求格式错误（非JSON）"
	description := "非JSON请求体，应返回400"
	expectedHTTPStatus := http.StatusBadRequest
	expectedMsg := "参数绑定失败"

	total++
	colorCase.Printf("\n用例 %d: %s\n", total, testName)
	fmt.Println("----------------------------------------")
	colorInfo.Printf("描述: %s\n", description)
	colorInfo.Println("请求体内容: invalid-json-format")

	resp, respBody, err := sendPostRequest(baseURL, adminToken, []byte("invalid-json-format"))
	if err != nil {
		colorFail.Printf("❌ 发送请求失败: %v\n", err)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

	casePassed := true
	if actualHTTPStatus != expectedHTTPStatus {
		colorFail.Printf("❌ 状态码错误: 预期 %d, 实际 %d\n", expectedHTTPStatus, actualHTTPStatus)
		casePassed = false
	} else {
		colorPass.Printf("✅ 状态码正确: %d\n", actualHTTPStatus)
	}

	if !strings.Contains(respMsg, expectedMsg) {
		colorFail.Printf("❌ 消息错误: 预期包含「%s」, 实际「%s」\n", expectedMsg, respMsg)
		casePassed = false
	} else {
		colorPass.Printf("✅ 消息正确: %s\n", respMsg)
	}

	if casePassed {
		colorPass.Println("----------------------------------------")
		colorPass.Printf("用例执行通过 ✅\n")
		passed++
	} else {
		colorFail.Println("----------------------------------------")
		colorFail.Printf("用例执行失败 ❌\n")
		failed++
	}
}

// 10. 邮箱格式不正确（无@）
func TestCreateUser_InvalidEmail(t *testing.T) {
	req := UserRequest{
		Email:     "notanemail", // 无效邮箱（无@）
		Password:  "ValidPass123!",
		Nickname:  "BadEmailNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = fmt.Sprintf("bademailuser%d", time.Now().Unix())
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"邮箱格式不正确（无@）",
		"邮箱不合法，应返回422",
		req,
		adminToken,
		http.StatusUnprocessableEntity,
		"Email must be a valid email address",
	)
}

// ==================== 全用例入口 ====================
func TestCreateUser_AllCases(t *testing.T) {
	// 添加「开始执行」提示
	fmt.Println("================================================================================")
	colorBlue.Println("开始执行创建用户接口测试用例")
	fmt.Println("================================================================================")

	// 重置统计计数
	total, passed, failed = 0, 0, 0
	defer cleanupTemp()

	// 按顺序执行所有10个用例
	t.Run("Test1_ValidParams", TestCreateUser_ValidParams)
	t.Run("Test2_DuplicateUsername", TestCreateUser_DuplicateUsername)
	t.Run("Test3_MissingRequiredField", TestCreateUser_MissingRequiredField)
	t.Run("Test4_InvalidUsername", TestCreateUser_InvalidUsername)
	t.Run("Test5_WeakPassword", TestCreateUser_WeakPassword)
	t.Run("Test6_NoAuthHeader", TestCreateUser_NoAuthHeader)
	t.Run("Test7_InvalidToken", TestCreateUser_InvalidToken)
	t.Run("Test8_Forbidden", TestCreateUser_Forbidden)
	t.Run("Test9_NonJSONBody", TestCreateUser_NonJSONBody)
	t.Run("Test10_InvalidEmail", TestCreateUser_InvalidEmail)

	// 测试总结
	fmt.Println("\n================================================================================")
	fmt.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d\n", total, passed, failed)
	fmt.Println("================================================================================")

	if failed > 0 {
		colorFail.Println("❌ 存在失败用例，请检查问题后重试!")
		t.Fatalf("共有 %d 个用例失败", failed)
	} else {
		colorPass.Println("🎉 所有测试用例全部通过!")
	}
}
