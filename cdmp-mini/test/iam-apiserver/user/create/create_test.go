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

// ==================== 全局配置（新增登录相关配置，删除硬编码Token） ====================
var (
	baseURL  = "http://localhost:8080/v1/users"
	loginURL = "http://localhost:8080/login" // 登录接口URL（需根据实际项目调整）
	tempDir  = "./temp_json"

	// 测试账号（根据实际测试环境配置，建议从配置文件读取，此处暂用固定值）
	adminUser  = "admin"           // 管理员用户名
	adminPass  = "Admin@2021"      // 管理员密码（需替换为实际密码）
	normalUser = "gettest-user105" // 普通用户用户名
	normalPass = "TestPass123!"    // 普通用户密码（需替换为实际密码）

	// 动态生成的Token（替代原硬编码，测试前自动赋值）
	adminToken string
	userToken  string

	// 原生ANSI颜色码
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

// ==================== 新增：登录相关结构体（需根据实际登录接口格式调整） ====================
// LoginRequest 登录请求体（字段名需与登录接口的参数名完全一致）
type LoginRequest struct {
	Username string `json:"username"` // 登录接口的“用户名”参数名（如不同需改，例：userName）
	Password string `json:"password"` // 登录接口的“密码”参数名（如不同需改，例：passWord）
}

// LoginResponse 登录响应体（字段名需与登录接口的返回格式完全一致）
type LoginResponse struct {
	Code    int    `json:"code"`    // 业务状态码（0=成功，匹配你的响应）
	Message string `json:"message"` // 提示信息（你的响应中为空，不影响）
	Data    struct {
		Token  string `json:"token"`  // 修正：与实际返回的"token"字段匹配
		Expire string `json:"expire"` // 修正：与实际返回的"expire"字段匹配（时间字符串）
	} `json:"data"`
}

// ==================== 工具函数（不变，新增Token自动获取函数） ====================
// generateUniqueID 生成唯一ID（毫秒级，避免重复）
func generateUniqueID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano()/1e6)
}

func initTempDir() error {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return fmt.Errorf("创建临时目录失败: %w", err)
		}
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

// sendPostRequest 通用POST请求发送函数（复用给“登录”和“创建用户”接口）
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

// ==================== 新增：Token自动获取函数 ====================
// getToken 调用登录接口，根据用户名密码获取有效Token
func getToken(username, password string) (string, error) {
	// 1. 构建登录请求体
	loginReq := LoginRequest{
		Username: username,
		Password: password,
	}
	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("登录请求JSON序列化失败: %w", err)
	}

	// 2. 发送登录请求（登录接口无需Token，token参数传空）
	fmt.Printf("%s[INFO] 正在为用户「%s」获取Token...%s\n", ansiBlue, username, ansiReset)
	resp, respBody, err := sendPostRequest(loginURL, "", reqBody)
	if err != nil {
		return "", fmt.Errorf("发送登录请求失败: %w", err)
	}

	// 3. 校验登录请求的HTTP状态码（通常登录接口成功返回200 OK）
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"登录请求HTTP状态码错误: 预期200，实际%d，响应内容: %s",
			resp.StatusCode, string(respBody),
		)
	}

	// 4. 解析登录响应，提取Token
	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", fmt.Errorf(
			"登录响应JSON解析失败: %w，响应内容: %s",
			err, string(respBody),
		)
	}

	// 5. 校验业务状态（根据实际接口的成功码调整，例：code=0为成功）
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

	// 7. 返回Token（附带有效期提示）
	fmt.Printf(
		"%s[INFO] 用户「%s」Token获取成功，有效期%v秒%s\n",
		ansiGreen, username, loginResp.Data.Expire, ansiReset,
	)
	return loginResp.Data.Token, nil
}

// ==================== 测试执行函数（不变，Token来源改为自动获取） ====================
func runTestCase(t *testing.T, testName, description string, req UserRequest, token string, expectedHTTPStatus int, expectedMsg string) {
	// 原有逻辑不变...
	total++
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	fmt.Printf("%s描述: %s%s\n", ansiBlue, description, ansiReset)
	fmt.Printf("%s请求体JSON内容（语法校验后）:%s\n", ansiBlue, ansiReset)

	jsonFile, err := saveJSONToTemp(fmt.Sprintf("test%d.json", total), req)
	if err != nil {
		fmt.Printf("%s❌ %v%s\n", ansiRed, err, ansiReset)
		t.Fatalf("用例「%s」准备失败: %v", testName, err)
	}
	defer os.Remove(jsonFile)

	jsonContent, _ := os.ReadFile(jsonFile)
	fmt.Println(string(jsonContent))

	resp, respBody, err := sendPostRequest(baseURL, token, jsonContent)
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

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

// ==================== 10个用例的内部实现（不变，Token参数自动传递） ====================
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

// 1. 用例1：使用正确参数创建用户（用管理员Token）
func caseValidParams(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusCreated,
		"用户创建成功",
	)
}

// 2. 用例2：创建已存在的用户（用管理员Token）
func caseDuplicateUsername(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusConflict,
		"用户已经存在",
	)
}

// 3. 用例3：缺少必填字段（metadata.name）（用管理员Token）
func caseMissingRequiredField(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusUnprocessableEntity,
		"名称部分不能为空",
	)
}

// 4. 用例4：用户名不合法（含@）（用管理员Token）
func caseInvalidUsername(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusUnprocessableEntity,
		"名称部分必须由字母、数字、'-'、'_'或'.'组成",
	)
}

// 5. 用例5：密码不符合规则（弱密码123）（用管理员Token）
func caseWeakPassword(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusUnprocessableEntity,
		"密码设定不符合规则",
	)
}

// 6. 用例6：未提供Authorization头（无需Token）
func caseNoAuthHeader(t *testing.T) {
	// 原有逻辑不变...
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
		"", // 无需Token
		http.StatusUnauthorized,
		"缺少 Authorization 头",
	)
}

// 7. 用例7：使用无效token（用假Token）
func caseInvalidToken(t *testing.T) {
	// 原有逻辑不变...
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
		"invalid-token-123", // 假Token
		http.StatusUnauthorized,
		"token contains an invalid number of segments",
	)
}

// 8. 用例8：权限不足（普通用户创建用户）（用普通用户Token）
func caseForbidden(t *testing.T) {
	// 原有逻辑不变...
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
		userToken, // 自动获取的普通用户Token
		http.StatusForbidden,
		"权限不足",
	)
}

// 9. 用例9：请求格式错误（非JSON）（用管理员Token）
func caseNonJSONBody(t *testing.T) {
	// 原有逻辑不变...
	testName := "请求格式错误（非JSON）"
	description := "非JSON请求体，应返回400"
	expectedHTTPStatus := http.StatusBadRequest
	expectedMsg := "参数绑定失败"

	total++
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	fmt.Printf("%s描述: %s%s\n", ansiBlue, description, ansiReset)
	fmt.Printf("%s请求体内容: invalid-json-format%s\n", ansiBlue, ansiReset)

	resp, respBody, err := sendPostRequest(baseURL, adminToken, []byte("invalid-json-format"))
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	respCode, respMsg, fullResp := parseResponse(respBody)
	actualHTTPStatus := resp.StatusCode

	fmt.Printf("实际返回: code=%d message=%s\n", respCode, respMsg)
	fmt.Printf("完整响应结果: %s\n", fullResp)

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

// 10. 用例10：邮箱格式不正确（无@）（用管理员Token）
func caseInvalidEmail(t *testing.T) {
	// 原有逻辑不变...
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
		adminToken, // 自动获取的管理员Token
		http.StatusUnprocessableEntity,
		"Email must be a valid email address",
	)
}

// ==================== 唯一测试入口（新增Token自动初始化） ====================
func TestCreateUser_AllCases(t *testing.T) {
	// 第一步：自动获取Token（所有用例执行前必须完成）
	var err error
	// 1. 获取管理员Token
	adminToken, err = getToken(adminUser, adminPass)
	if err != nil {
		fmt.Printf("%s❌ 获取管理员Token失败: %v%s\n", ansiRed, err, ansiReset)
		t.Fatal("管理员Token获取失败，测试终止")
	}
	// 2. 获取普通用户Token
	userToken, err = getToken(normalUser, normalPass)
	if err != nil {
		fmt.Printf("%s❌ 获取普通用户Token失败: %v%s\n", ansiRed, err, ansiReset)
		t.Fatal("普通用户Token获取失败，测试终止")
	}

	// 第二步：验证Token有效性（双重保障）
	if adminToken == "" || userToken == "" {
		fmt.Printf("%s❌ 自动获取的Token为空，测试终止%s\n", ansiRed, ansiReset)
		t.Fatal("Token为空，测试终止")
	}
	fmt.Printf("%s[INFO] 所有Token初始化完成，开始执行测试用例...%s\n", ansiGreen, ansiReset)

	// 第三步：执行原有测试逻辑
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

	// 测试总结
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
