package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/golang-jwt/jwt/v4"
)

// 关键修复：初始化颜色时强制启用颜色输出（解决终端不兼容问题）
var (
	colorPass    = initColor(color.New(color.FgGreen).Add(color.Bold))
	colorFail    = initColor(color.New(color.FgRed).Add(color.Bold))
	colorInfo    = initColor(color.New(color.FgCyan))
	colorCase    = initColor(color.New(color.FgYellow).Add(color.Bold))
	colorCode    = initColor(color.New(color.FgMagenta).Add(color.Bold))
	colorCode400 = initColor(color.New(color.FgYellow).Add(color.Bold))
)

// 初始化颜色并确保颜色启用的兼容函数
func initColor(c *color.Color) *color.Color {
	c.EnableColor()
	color.NoColor = false
	return c
}

// 全局统计变量（添加互斥锁，避免并行执行时竞态）
var (
	total   int
	passed  int
	failed  int
	results []string
	mu      sync.Mutex // 新增：统计变量的互斥锁
)

// 配置常量（修复Token硬编码问题，新增JWT密钥）
const (
	// API基础信息
	BASE_URL    = "http://localhost:8080"
	API_VERSION = "v1"

	// 测试数据
	VALID_USER    = "gettest-user104"
	ADMIN_USER    = "admin"
	INVALID_USER  = "non-existent-user-999"
	INVALID_ROUTE = "invalid-route-123"

	// 新增：JWT签名密钥（需与服务器一致！）
	JWT_SECRET = "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo" // 替换为你服务器的
	// 真实JWT密钥
	// Token有效期（测试用设为1小时，足够覆盖测试）
	TOKEN_EXPIRE = 1 * time.Hour

	// 业务错误码（保持不变）
	ERR_SUCCESS_CODE        = 0
	ERR_PAGE_NOT_FOUND      = 100005
	ERR_METHOD_NOT_ALLOWED  = 100006
	ERR_MISSING_HEADER      = 100205
	ERR_INVALID_AUTH_HEADER = 100204
	ERR_TOKEN_INVALID       = 100208
	ERR_EXPIRED             = 100203
	ERR_PERMISSION_DENIED   = 100207
	ERR_USER_NOT_FOUND      = 110001
)

// 新增：JWT Claims结构（需与服务器一致）
type JwtClaims struct {
	Identity string `json:"identity"` // 服务器用的身份字段（如管理员/普通用户）
	Sub      string `json:"sub"`      // 用户名/用户ID
	jwt.RegisteredClaims
}

// 新增：动态生成JWT Token（避免硬编码过期问题）
// tokenType: "admin"（管理员）、"normal"（普通用户）、"expired"（过期）
func generateToken(tokenType string) (string, error) {
	now := time.Now()
	var expireTime time.Time

	// 1. 先处理过期Token的时间（无论角色，过期时间都设为过去）
	if tokenType == "expired" {
		expireTime = now.Add(-1 * time.Hour) // 过期1小时（确保已过期）
	} else {
		expireTime = now.Add(TOKEN_EXPIRE) // 正常有效期
	}

	// 2. 设置不同角色的Claims（过期Token也需要正确的角色信息，只是时间过期）
	var identity, sub string
	switch tokenType {
	case "admin", "expired": // 过期Token可以复用管理员的身份信息（只是时间过期）
		identity = "admin"
		sub = ADMIN_USER
	case "normal":
		identity = "gettest-user104"
		sub = VALID_USER
	default:
		return "", fmt.Errorf("无效的token类型：%s", tokenType)
	}

	// 3. 生成Claims并签名
	claims := JwtClaims{
		Identity: identity,
		Sub:      sub,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "iam-apiserver", // 与服务器一致
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWT_SECRET))
}

// 发送HTTP GET请求（保持不变）
func sendGetRequest(url, token string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败：%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// 解析响应体（保持不变）
func parseRespBody(resp *http.Response, caseID string) (map[string]interface{}, error) {
	defer resp.Body.Close()
	if resp.Body == nil {
		return nil, fmt.Errorf("[%s] 响应体为空", caseID)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[%s] 读取响应体失败：%v", caseID, err)
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	colorInfo.Println(fmt.Sprintf("[%s] 原始响应体：%s", caseID, string(bodyBytes)))
	color.Unset()

	var respBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
		return nil, fmt.Errorf("[%s] JSON解析失败：%v，原始内容：%s", caseID, err, string(bodyBytes))
	}
	return respBody, nil
}

// 获取响应中的业务码（保持不变）
func getResponseCode(respBody map[string]interface{}, caseID string) (int, error) {
	if codeVal, ok := respBody["code"]; ok {
		if codeNum, ok := codeVal.(float64); ok {
			return int(codeNum), nil
		}
		if codeStr, ok := codeVal.(string); ok {
			codeNum, err := strconv.Atoi(codeStr)
			if err != nil {
				return 0, fmt.Errorf("[%s] code字段转换失败：%s", caseID, codeStr)
			}
			return codeNum, nil
		}
		return 0, fmt.Errorf("[%s] code字段类型不支持：%T", caseID, codeVal)
	}
	return 0, fmt.Errorf("[%s] 响应体无code字段", caseID)
}

// 记录用例结果（新增互斥锁，避免并行竞态）
func recordResult(caseID, caseName string, isPassed bool, failReason string) {
	mu.Lock()
	defer mu.Unlock()

	total++
	if isPassed {
		passed++
		results = append(results, fmt.Sprintf("%s：%s → 执行通过 ✅", caseID, caseName))
	} else {
		failed++
		results = append(results, fmt.Sprintf("%s：%s → 执行失败 ❌（失败原因：%s）", caseID, caseName, failReason))
	}
}

// 打印用例结果（保持不变）
func printCaseResult(caseID, caseName string, isPassed bool, failReason string) {
	fmt.Println(strings.Repeat("=", 80))
	if isPassed {
		colorPass.Println(fmt.Sprintf("[%s] %s → 执行通过 ✅", caseID, caseName))
	} else {
		colorFail.Println(fmt.Sprintf("[%s] %s → 执行失败 ❌", caseID, caseName))
		colorFail.Println(fmt.Sprintf("[%s] 失败原因：%s", caseID, failReason))
	}
	color.Unset()
}

// 打印测试汇总报告（保持不变）
func printTestSummary() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	colorInfo.Println("GET接口测试汇总报告")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("总用例数：%d\n", total)
	colorPass.Println(fmt.Sprintf("通过用例：%d", passed))
	colorFail.Println(fmt.Sprintf("失败用例：%d", failed))
	fmt.Println(strings.Repeat("-", 80))

	colorInfo.Println("用例执行详情：")
	for _, result := range results {
		if strings.Contains(result, "执行失败") {
			colorFail.Println(result)
		} else {
			colorPass.Println(result)
		}
	}
	fmt.Println(strings.Repeat("=", 80))
	color.Unset()

	if failed > 0 {
		colorFail.Println("\n排查建议：")
		colorFail.Println("1. HTTP 401错误（未认证）：")
		colorFail.Println("   - 确认JWT_SECRET是否与服务器一致")
		colorFail.Println("   - 检查generateToken函数的Claims是否与服务器匹配")
		colorFail.Println("2. HTTP 404错误（资源不存在）：")
		colorFail.Println("   - 确认请求URL是否正确（如用户ID、路由路径）")
		colorFail.Println("3. 执行单个用例命令示例：")
		colorFail.Println("   - 执行用例8：go test -v -run \"TestUserGetAPI_GET008\" get_test.go")
		colorFail.Println("   - 执行所有用例：go test -v -run \"TestUserGetAPI_All\" get_test.go")
		color.Unset()
	} else {
		colorPass.Println("\n🎉 所有测试用例全部通过！")
		color.Unset()
	}
}

// 用例1：管理员查询普通用户（有权限）→ 修复Token生成
func TestUserGetAPI_GET001(t *testing.T) {
	caseID := "GET-001"
	caseName := "管理员查询普通用户（有权限）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	// 修复：动态生成管理员Token（避免过期）
	adminToken, err := generateToken("admin")
	if err != nil {
		failReason := fmt.Sprintf("生成管理员Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + adminToken // 拼接Bearer前缀
	expectedStatus := http.StatusOK
	expectedCode := ERR_SUCCESS_CODE

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s...", token[:35]))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	// 修复：字段名兼容（先检查小写username，再检查大写Username）
	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		failReason := "响应体缺少data字段或格式错误"
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	// 兼容服务器返回的字段名（小写username或大写Username）
	var actualUser string
	if userVal, ok := data["username"]; ok {
		actualUser = fmt.Sprintf("%v", userVal)
	} else if userVal, ok := data["Username"]; ok {
		actualUser = fmt.Sprintf("%v", userVal)
	} else {
		failReason := "data字段中缺少username/Username字段"
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	if actualUser != VALID_USER {
		failReason := fmt.Sprintf("用户名不匹配：预期%s，实际%s", VALID_USER, actualUser)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例2：管理员查询不存在用户→ 修复Token生成
func TestUserGetAPI_GET002(t *testing.T) {
	caseID := "GET-002"
	caseName := "管理员查询不存在用户"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, INVALID_USER)
	// 修复：动态生成管理员Token
	adminToken, err := generateToken("admin")
	if err != nil {
		failReason := fmt.Sprintf("生成管理员Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + adminToken
	expectedStatus := http.StatusNotFound
	expectedCode := ERR_USER_NOT_FOUND

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例3：缺少Authorization请求头→ 无需修改（无Token）
func TestUserGetAPI_GET003(t *testing.T) {
	caseID := "GET-003"
	caseName := "缺少Authorization请求头"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	token := ""
	expectedStatus := http.StatusUnauthorized
	expectedCode := ERR_MISSING_HEADER

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println("Token：无")
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例4：Authorization格式无效（无Bearer前缀）→ 无需修改（固定无效格式）
func TestUserGetAPI_GET004(t *testing.T) {
	caseID := "GET-004"
	caseName := "Authorization格式无效（无Bearer前缀）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	// 修复：动态生成有效Token但去掉Bearer前缀（确保格式无效，内容有效）
	normalToken, err := generateToken("normal")
	if err != nil {
		failReason := fmt.Sprintf("生成普通用户Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := normalToken // 无Bearer前缀（格式无效）
	expectedStatus := http.StatusBadRequest
	expectedCode := ERR_INVALID_AUTH_HEADER

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s", token[:20]+"..."))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例5：Token格式正确但内容无效（签名错误）→ 修复无效Token生成
func TestUserGetAPI_GET005(t *testing.T) {
	caseID := "GET-005"
	caseName := "Token格式正确但内容无效（签名错误）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	// 修复：用错误密钥生成Token（确保签名错误，格式正确）
	wrongSecret := "wrong-jwt-secret-456" // 与服务器密钥不一致
	now := time.Now()
	claims := JwtClaims{
		Identity: "normal",
		Sub:      VALID_USER,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(TOKEN_EXPIRE)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	// 用错误密钥签名
	invalidToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(wrongSecret))
	if err != nil {
		failReason := fmt.Sprintf("生成无效Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + invalidToken // 格式正确，内容无效
	expectedStatus := http.StatusUnauthorized
	expectedCode := ERR_TOKEN_INVALID

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s...", token[:35]))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例6：使用过期Token查询→ 修复过期Token生成
func TestUserGetAPI_GET006(t *testing.T) {
	caseID := "GET-006"
	caseName := "使用过期Token查询"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	// 修复：动态生成过期Token（无需硬编码）
	expiredToken, err := generateToken("expired")
	if err != nil {
		failReason := fmt.Sprintf("生成过期Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + expiredToken
	expectedStatus := http.StatusUnauthorized
	expectedCode := ERR_EXPIRED

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s...", token[:35]))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例7：普通用户查询管理员（无权限）→ 修复普通用户Token生成
func TestUserGetAPI_GET007(t *testing.T) {
	caseID := "GET-007"
	caseName := "普通用户查询管理员（无权限）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, ADMIN_USER)
	// 修复：动态生成普通用户Token
	normalToken, err := generateToken("normal")
	if err != nil {
		failReason := fmt.Sprintf("生成普通用户Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + normalToken
	expectedStatus := http.StatusForbidden
	expectedCode := ERR_PERMISSION_DENIED

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s...", token[:35]))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例8：访问不存在的路由（非/users路径）→ 修复预期状态码和业务码
func TestUserGetAPI_GET008(t *testing.T) {
	caseID := "GET-008"
	caseName := "访问不存在的路由（非/users路径）"
	reqURL := fmt.Sprintf("%s/%s/%s", BASE_URL, API_VERSION, INVALID_ROUTE)
	// 修复：动态生成管理员Token
	adminToken, err := generateToken("admin")
	if err != nil {
		failReason := fmt.Sprintf("生成管理员Token失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	token := "Bearer " + adminToken
	// 修复：不存在的路由预期404 + 100005（原预期405+100006错误）
	expectedStatus := http.StatusNotFound
	expectedCode := ERR_PAGE_NOT_FOUND

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s...", token[:35]))
	color.Unset()

	resp, err := sendGetRequest(reqURL, token)
	if err != nil {
		failReason := fmt.Sprintf("请求发送失败：%v", err)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	actualStatus := resp.StatusCode
	colorInfo.Println(fmt.Sprintf("[%s] 实际响应：HTTP %d", caseID, actualStatus))
	color.Unset()

	if actualStatus != expectedStatus {
		failReason := fmt.Sprintf("HTTP状态码不匹配：预期%d，实际%d", expectedStatus, actualStatus)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	respBody, parseErr := parseRespBody(resp, caseID)
	if parseErr != nil {
		failReason := parseErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	actualCode, codeErr := getResponseCode(respBody, caseID)
	if codeErr != nil {
		failReason := codeErr.Error()
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}
	if actualCode != expectedCode {
		failReason := fmt.Sprintf("业务码不匹配：预期%d，实际%d", expectedCode, actualCode)
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 执行所有用例的入口（保持不变）
func TestUserGetAPI_All(t *testing.T) {
	// 重置统计变量
	mu.Lock()
	total = 0
	passed = 0
	failed = 0
	results = []string{}
	mu.Unlock()

	// 执行所有用例
	t.Run("GET-001", TestUserGetAPI_GET001)
	t.Run("GET-002", TestUserGetAPI_GET002)
	t.Run("GET-003", TestUserGetAPI_GET003)
	t.Run("GET-004", TestUserGetAPI_GET004)
	t.Run("GET-005", TestUserGetAPI_GET005)
	t.Run("GET-006", TestUserGetAPI_GET006)
	t.Run("GET-007", TestUserGetAPI_GET007)
	t.Run("GET-008", TestUserGetAPI_GET008)

	// 打印汇总报告
	printTestSummary()
}

// 支持直接运行（go run）
func main() {
	testing.Main(
		func(pat, str string) (bool, error) { return true, nil },
		[]testing.InternalTest{
			{Name: "TestUserGetAPI_GET001", F: TestUserGetAPI_GET001},
			{Name: "TestUserGetAPI_GET002", F: TestUserGetAPI_GET002},
			{Name: "TestUserGetAPI_GET003", F: TestUserGetAPI_GET003},
			{Name: "TestUserGetAPI_GET004", F: TestUserGetAPI_GET004},
			{Name: "TestUserGetAPI_GET005", F: TestUserGetAPI_GET005},
			{Name: "TestUserGetAPI_GET006", F: TestUserGetAPI_GET006},
			{Name: "TestUserGetAPI_GET007", F: TestUserGetAPI_GET007},
			{Name: "TestUserGetAPI_GET008", F: TestUserGetAPI_GET008},
			{Name: "TestUserGetAPI_All", F: TestUserGetAPI_All},
		},
		nil, nil,
	)
}
