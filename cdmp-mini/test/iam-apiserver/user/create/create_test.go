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
)

// ==================== 全局配置（删除fatih/color依赖，改用ANSI颜色码） ====================
var (
	baseURL = "http://localhost:8080/v1/users"
	// ！！！必须替换为有效令牌！！！
	adminToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzIxODQ2OSwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTcxMzIwNjksInN1YiI6ImFkbWluIn0.DWXPUWVSf3Zh1QM3G6zyNU5FlVUOkGTAooZGS5DX-wE"                            // 管理员有效令牌
	userToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzIxODUxMSwiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDQiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdfaWF0IjoxNzU3MTMyMTExLCJzdWIiOiJnZXR0ZXN0LXVzZXIxMDQifQ.jzHM7hZBJL9e1WLBAkAtDf8KkMIXFXw0PfkwBhn8kko" // 普通用户有效令牌
	tempDir    = "./temp_json"

	// 原生ANSI颜色码（兼容性强，所有终端通用）
	ansiReset  = "\033[0m"    // 重置颜色
	ansiGreen  = "\033[32;1m" // 绿色加粗（通过提示）
	ansiRed    = "\033[31;1m" // 红色加粗（失败提示）
	ansiBlue   = "\033[34m"   // 蓝色（信息提示）
	ansiYellow = "\033[33;1m" // 黄色加粗（用例标题）

	// 测试统计
	total  int
	passed int
	failed int
)

// ==================== 工具函数（替换彩色输出为原生ANSI码） ====================
// 生成唯一ID（毫秒级，避免重复）
func generateUniqueID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano()/1e6)
}

func initTempDir() error {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return fmt.Errorf("创建临时目录失败: %w", err)
		}
		// 蓝色信息提示（原生ANSI码）
		fmt.Printf("%s[INFO] 临时目录创建成功: %s%s\n", ansiBlue, tempDir, ansiReset)
	}
	return nil
}

func generateInstanceID() string {
	return generateUniqueID("usr")
}

func cleanupTemp() {
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		if err := os.RemoveAll(tempDir); err != nil {
			fmt.Printf("%s[INFO] 临时文件清理失败: %v%s\n", ansiBlue, err, ansiReset)
			return
		}
		fmt.Printf("%s[INFO] 临时文件已清理%s\n", ansiBlue, ansiReset)
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

// ==================== 测试执行函数（原生ANSI彩色输出） ====================
func runTestCase(t *testing.T, testName, description string, req UserRequest, token string, expectedHTTPStatus int, expectedMsg string) {
	total++
	// 黄色加粗：用例标题
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	// 蓝色：描述信息
	fmt.Printf("%s描述: %s%s\n", ansiBlue, description, ansiReset)
	fmt.Printf("%s请求体JSON内容（语法校验后）:%s\n", ansiBlue, ansiReset)

	// 保存请求体到临时文件
	jsonFile, err := saveJSONToTemp(fmt.Sprintf("test%d.json", total), req)
	if err != nil {
		// 红色：错误提示
		fmt.Printf("%s❌ %v%s\n", ansiRed, err, ansiReset)
		t.Fatalf("用例「%s」准备失败: %v", testName, err)
	}
	defer os.Remove(jsonFile)

	// 读取并打印JSON内容
	jsonContent, _ := os.ReadFile(jsonFile)
	fmt.Println(string(jsonContent))

	// 发送请求
	resp, respBody, err := sendPostRequest(baseURL, token, jsonContent)
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	// 解析响应
	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	// 打印响应信息
	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

	// 验证结果
	casePassed := true
	// 状态码验证
	if actualHTTPStatus == expectedHTTPStatus {
		fmt.Printf("%s✅ 状态码正确: %d%s\n", ansiGreen, actualHTTPStatus, ansiReset)
	} else {
		fmt.Printf("%s❌ 状态码错误: 预期 %d, 实际 %d%s\n", ansiRed, expectedHTTPStatus, actualHTTPStatus, ansiReset)
		casePassed = false
	}

	// 消息验证
	if strings.Contains(respMsg, expectedMsg) {
		fmt.Printf("%s✅ 消息正确: %s%s\n", ansiGreen, respMsg, ansiReset)
	} else {
		fmt.Printf("%s❌ 消息错误: 预期包含「%s」, 实际「%s」%s\n", ansiRed, expectedMsg, respMsg, ansiReset)
		casePassed = false
	}

	// 统计结果
	if casePassed {
		fmt.Printf("%s----------------------------------------%s\n", ansiGreen, ansiReset)
		fmt.Printf("%s✅ 用例执行通过 %s\n", ansiGreen, ansiReset)
		passed++
	} else {
		fmt.Printf("%s----------------------------------------%s\n", ansiRed, ansiReset)
		fmt.Printf("%s❌ 用例执行失败 %s\n", ansiRed, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败", testName)
	}
}

// ==================== 10个用例的内部实现（不变） ====================
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

// 1. 用例1：使用正确参数创建用户
func caseValidParams(t *testing.T) {
	uniqueUsername := generateUniqueID("testuser")
	uniqueEmail := generateUniqueID("test") + "@example.com"

	req := UserRequest{
		Email:     uniqueEmail,
		Password:  "ValidPass123!",
		Nickname:  "TestUserNickname",
		Phone:     "13800138000",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = uniqueUsername
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

// 2. 用例2：创建已存在的用户
func caseDuplicateUsername(t *testing.T) {
	// 先创建基础用户
	baseUsername := generateUniqueID("duplicateuser")
	baseEmail := generateUniqueID("duplicate") + "@example.com"
	baseReq := UserRequest{
		Email:     baseEmail,
		Password:  "ValidPass123!",
		Nickname:  "BaseUserNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	baseReq.Metadata.Name = baseUsername
	baseReq.Metadata.InstanceID = generateInstanceID()
	baseReq.Metadata.Extend = make(map[string]interface{})
	baseJSON, _ := json.Marshal(baseReq)
	sendPostRequest(baseURL, adminToken, baseJSON)

	// 重复创建
	dupReq := UserRequest{
		Email:     generateUniqueID("anotherduplicate") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "DuplicateNickname",
		Phone:     "13900139000",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	dupReq.Metadata.Name = baseUsername
	dupReq.Metadata.InstanceID = generateInstanceID()
	dupReq.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"创建已存在的用户",
		"用户名重复，应返回409",
		dupReq,
		adminToken,
		http.StatusConflict,
		"用户已经存在",
	)
}

// 3. 用例3：缺少必填字段（metadata.name）
func caseMissingRequiredField(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("missingname") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "MissingNameNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
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

// 4. 用例4：用户名不合法（含@）
func caseInvalidUsername(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("invalidname") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "InvalidNameNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = "invalid@username"
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

// 5. 用例5：密码不符合规则（弱密码123）
func caseWeakPassword(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("weakpass") + "@example.com",
		Password:  "123",
		Nickname:  "WeakPassNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = generateUniqueID("weakpassuser")
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

// 6. 用例6：未提供Authorization头
func caseNoAuthHeader(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("noauth") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "NoAuthNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = generateUniqueID("noauthuser")
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"未提供Authorization头",
		"无认证令牌，应返回401",
		req,
		"",
		http.StatusUnauthorized,
		"缺少 Authorization 头",
	)
}

// 7. 用例7：使用无效token
func caseInvalidToken(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("invalidtoken") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "InvalidTokenNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = generateUniqueID("invalidtokenuser")
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"使用无效token",
		"令牌格式错误，应返回401",
		req,
		"invalid-token",
		http.StatusUnauthorized,
		"token contains an invalid number of segments",
	)
}

// 8. 用例8：权限不足（普通用户创建用户）
func caseForbidden(t *testing.T) {
	req := UserRequest{
		Email:     generateUniqueID("forbidden") + "@example.com",
		Password:  "ValidPass123!",
		Nickname:  "ForbiddenNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = generateUniqueID("forbiddenuser")
	req.Metadata.InstanceID = generateInstanceID()
	req.Metadata.Extend = make(map[string]interface{})

	runTestCase(
		t,
		"权限不足（普通用户创建用户）",
		"普通用户无创建权限，应返回403",
		req,
		userToken,
		http.StatusForbidden,
		"权限不足",
	)
}

// 9. 用例9：请求格式错误（非JSON）
func caseNonJSONBody(t *testing.T) {
	testName := "请求格式错误（非JSON）"
	description := "非JSON请求体，应返回400"
	expectedHTTPStatus := http.StatusBadRequest
	expectedMsg := "参数绑定失败"

	total++
	// 黄色加粗：用例标题
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	// 蓝色：描述信息
	fmt.Printf("%s描述: %s%s\n", ansiBlue, description, ansiReset)
	fmt.Printf("%s请求体内容: invalid-json-format%s\n", ansiBlue, ansiReset)

	// 发送非JSON请求
	resp, respBody, err := sendPostRequest(baseURL, adminToken, []byte("invalid-json-format"))
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	// 解析响应
	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	// 打印响应信息
	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

	// 验证结果
	casePassed := true
	if actualHTTPStatus == expectedHTTPStatus {
		fmt.Printf("%s✅ 状态码正确: %d%s\n", ansiGreen, actualHTTPStatus, ansiReset)
	} else {
		fmt.Printf("%s❌ 状态码错误: 预期 %d, 实际 %d%s\n", ansiRed, expectedHTTPStatus, actualHTTPStatus, ansiReset)
		casePassed = false
	}

	if strings.Contains(respMsg, expectedMsg) {
		fmt.Printf("%s✅ 消息正确: %s%s\n", ansiGreen, respMsg, ansiReset)
	} else {
		fmt.Printf("%s❌ 消息错误: 预期包含「%s」, 实际「%s」%s\n", ansiRed, expectedMsg, respMsg, ansiReset)
		casePassed = false
	}

	// 统计结果
	if casePassed {
		fmt.Printf("%s----------------------------------------%s\n", ansiGreen, ansiReset)
		fmt.Printf("%s✅ 用例执行通过 %s\n", ansiGreen, ansiReset)
		passed++
	} else {
		fmt.Printf("%s----------------------------------------%s\n", ansiRed, ansiReset)
		fmt.Printf("%s❌ 用例执行失败 %s\n", ansiRed, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败", testName)
	}
}

// 10. 用例10：邮箱格式不正确（无@）
func caseInvalidEmail(t *testing.T) {
	req := UserRequest{
		Email:     "notanemail" + generateUniqueID(""),
		Password:  "ValidPass123!",
		Nickname:  "BadEmailNickname",
		Status:    1,
		LoginedAt: "2024-09-05T12:00:00Z",
	}
	req.Metadata.Name = generateUniqueID("bademailuser")
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

// ==================== 唯一测试入口（原生ANSI彩色提示） ====================
func TestCreateUser_AllCases(t *testing.T) {
	// 验证令牌是否已替换
	if adminToken == "REPLACE_WITH_YOUR_VALID_ADMIN_TOKEN" || userToken == "REPLACE_WITH_YOUR_VALID_USER_TOKEN" {
		fmt.Printf("%s❌ 请先替换代码中的 adminToken 和 userToken 为有效令牌！%s\n", ansiRed, ansiReset)
		t.Fatal("令牌未替换，测试终止")
	}

	// 彩色开始提示
	fmt.Println("================================================================================")
	fmt.Printf("%s开始执行创建用户接口测试用例%s\n", ansiBlue, ansiReset)
	fmt.Println("================================================================================")

	total, passed, failed = 0, 0, 0
	defer cleanupTemp()

	// 执行所有用例
	t.Run("用例1_正确参数创建用户", func(t *testing.T) { caseValidParams(t) })
	t.Run("用例2_重复用户名", func(t *testing.T) { caseDuplicateUsername(t) })
	t.Run("用例3_缺少用户名", func(t *testing.T) { caseMissingRequiredField(t) })
	t.Run("用例4_用户名含@", func(t *testing.T) { caseInvalidUsername(t) })
	t.Run("用例5_弱密码", func(t *testing.T) { caseWeakPassword(t) })
	t.Run("用例6_无认证头", func(t *testing.T) { caseNoAuthHeader(t) })
	t.Run("用例7_无效token", func(t *testing.T) { caseInvalidToken(t) })
	t.Run("用例8_权限不足", func(t *testing.T) { caseForbidden(t) })
	t.Run("用例9_非JSON格式", func(t *testing.T) { caseNonJSONBody(t) })
	t.Run("用例10_无效邮箱", func(t *testing.T) { caseInvalidEmail(t) })

	// 彩色总结
	fmt.Println("\n================================================================================")
	fmt.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d\n", total, passed, failed)
	fmt.Println("================================================================================")

	if failed > 0 {
		fmt.Printf("%s❌ 存在失败用例，请检查问题后重试!%s\n", ansiRed, ansiReset)
		t.Fatalf("共有 %d 个用例失败", failed)
	} else {
		fmt.Printf("%s🎉 所有测试用例全部通过!%s\n", ansiGreen, ansiReset)
	}
}
