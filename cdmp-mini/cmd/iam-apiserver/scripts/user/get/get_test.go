package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
)

// 关键修复：初始化颜色时强制启用颜色输出（解决终端不兼容问题）
// 兼容所有版本的颜色配置
var (
	// 初始化颜色并强制开启颜色模式
	colorPass    = initColor(color.New(color.FgGreen).Add(color.Bold))
	colorFail    = initColor(color.New(color.FgRed).Add(color.Bold))
	colorInfo    = initColor(color.New(color.FgCyan))
	colorCase    = initColor(color.New(color.FgYellow).Add(color.Bold))
	colorCode    = initColor(color.New(color.FgMagenta).Add(color.Bold))
	colorCode400 = initColor(color.New(color.FgYellow).Add(color.Bold))
)

// 初始化颜色并确保颜色启用的兼容函数
func initColor(c *color.Color) *color.Color {
	// 对不同版本做兼容处理
	c.EnableColor()       // 尝试调用EnableColor()
	color.NoColor = false // 强制关闭无颜色模式
	return c
}

// 全局统计变量
var (
	total   int
	passed  int
	failed  int
	results []string // 存储每个用例的执行结果
)

// 配置常量（修复Token中的拼写错误）
const (
	// API基础信息
	BASE_URL    = "http://localhost:8080"
	API_VERSION = "v1"

	// 测试数据
	VALID_USER    = "gettest-user104"
	ADMIN_USER    = "admin"
	INVALID_USER  = "non-existent-user-999"
	INVALID_ROUTE = "invalid-route-123"

	// 认证Token（修复NO_PERMISSION_TOKEN的"Bearerarer"拼写错误）
	VALID_TOKEN           = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzE2Mjc0MCwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTcwNzYzNDAsInN1YiI6ImFkbWluIn0.zQ-NDeRDyCDeSc3uZSO3YYKO1SS2tzVuStapG22J0EM"
	NO_PERMISSION_TOKEN   = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzE2MzA4OCwiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDQiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdfaWF0IjoxNzU3MDc2Njg4LCJzdWIiOiJnZXR0ZXN0LXVzZXIxMDQifQ.REIjlW628JsELJkgPyiBvM51wltIl8rvR7PLNIkPn1s"
	EXPIRED_TOKEN         = "Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzA3OTgyMSwiaWRlbnRpdHkiOiJnZXR0ZXN0LXVzZXIxMDEiLCJpc3MiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsIm9yaWdpbl9pYXQiOjE3NTcwNzk4MjEsInN1YiI6ImdldHRlc3QtdXNlcjEwNCJ9.2ynNNWPl8q4I3yHkdebpgAY_QAQ0rX5nw1sEP5ru-Jg"
	INVALID_FORMAT_TOKEN  = "invalid_token_without_bearer"
	INVALID_CONTENT_TOKEN = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6ImJhc2ljX3Rva2VuXzEyMyJ9.xxx"

	// 业务错误码
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

// 发送HTTP GET请求
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

// 解析响应体
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
	// 修复：使用Println而非Printf，确保颜色生效
	colorInfo.Println(fmt.Sprintf("[%s] 原始响应体：%s", caseID, string(bodyBytes)))
	color.Unset()

	var respBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
		return nil, fmt.Errorf("[%s] JSON解析失败：%v，原始内容：%s", caseID, err, string(bodyBytes))
	}
	return respBody, nil
}

// 获取响应中的业务码
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

// 记录用例结果
func recordResult(caseID, caseName string, isPassed bool, failReason string) {
	total++
	if isPassed {
		passed++
		results = append(results, fmt.Sprintf("%s：%s → 执行通过 ✅", caseID, caseName))
	} else {
		failed++
		results = append(results, fmt.Sprintf("%s：%s → 执行失败 ❌（失败原因：%s）", caseID, caseName, failReason))
	}
}

// 打印用例结果（修复颜色输出逻辑）
func printCaseResult(caseID, caseName string, isPassed bool, failReason string) {
	fmt.Println(strings.Repeat("=", 80))
	if isPassed {
		// 修复：使用Println确保颜色渲染
		colorPass.Println(fmt.Sprintf("[%s] %s → 执行通过 ✅", caseID, caseName))
	} else {
		colorFail.Println(fmt.Sprintf("[%s] %s → 执行失败 ❌", caseID, caseName))
		colorFail.Println(fmt.Sprintf("[%s] 失败原因：%s", caseID, failReason))
	}
	color.Unset() // 强制重置颜色
}

// 打印测试汇总报告（修复颜色输出）
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
		colorFail.Println("1. HTTP 405错误（方法不允许）：")
		colorFail.Println(fmt.Sprintf("   - 确认路由「%s」是否仅支持POST/PUT等其他方法", fmt.Sprintf("%s/%s/%s", BASE_URL, API_VERSION, INVALID_ROUTE)))
		colorFail.Println("2. 执行单个用例命令示例：")
		colorFail.Println("   - 执行用例8：go test -v -run \"TestUserGetAPI_GET008\" get_test.go")
		colorFail.Println("   - 执行所有用例：go test -v -run \"TestUserGetAPI_All\" get_test.go")
		color.Unset()
	} else {
		colorPass.Println("\n🎉 所有测试用例全部通过！")
		color.Unset()
	}
}

// 用例1：管理员查询普通用户（有权限）
func TestUserGetAPI_GET001(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-001"
	caseName := "管理员查询普通用户（有权限）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	token := VALID_TOKEN
	expectedStatus := http.StatusOK
	expectedCode := ERR_SUCCESS_CODE

	// 修复：使用Println确保颜色生效
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

	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		failReason := "响应体缺少data字段或格式错误"
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	if data["Username"] != VALID_USER {
		failReason := fmt.Sprintf("用户名不匹配：预期%s，实际%s", VALID_USER, data["Username"])
		printCaseResult(caseID, caseName, false, failReason)
		recordResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printCaseResult(caseID, caseName, true, "")
	recordResult(caseID, caseName, true, "")
}

// 用例2：管理员查询不存在用户
func TestUserGetAPI_GET002(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-002"
	caseName := "管理员查询不存在用户"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, INVALID_USER)
	token := VALID_TOKEN
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

// 用例3：缺少Authorization请求头
func TestUserGetAPI_GET003(t *testing.T) {
	gin.SetMode(gin.TestMode)
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

// 用例4：Authorization格式无效（无Bearer前缀）
func TestUserGetAPI_GET004(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-004"
	caseName := "Authorization格式无效（无Bearer前缀）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	token := INVALID_FORMAT_TOKEN
	expectedStatus := http.StatusBadRequest
	expectedCode := ERR_INVALID_AUTH_HEADER

	colorCase.Println(fmt.Sprintf("[%s] 开始执行：%s", caseID, caseName))
	colorInfo.Println(fmt.Sprintf("请求URL：%s", reqURL))
	colorInfo.Println(fmt.Sprintf("预期：HTTP %d | 业务码 %d", expectedStatus, expectedCode))
	colorInfo.Println(fmt.Sprintf("Token：%s", token))
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

// 用例5：Token格式正确但内容无效（签名错误）
func TestUserGetAPI_GET005(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-005"
	caseName := "Token格式正确但内容无效（签名错误）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	token := INVALID_CONTENT_TOKEN
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

// 用例6：使用过期Token查询
func TestUserGetAPI_GET006(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-006"
	caseName := "使用过期Token查询"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, VALID_USER)
	token := EXPIRED_TOKEN
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

// 用例7：普通用户查询管理员（无权限）
func TestUserGetAPI_GET007(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-007"
	caseName := "普通用户查询管理员（无权限）"
	reqURL := fmt.Sprintf("%s/%s/users/%s", BASE_URL, API_VERSION, ADMIN_USER)
	token := NO_PERMISSION_TOKEN
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

// 用例8：访问不存在的路由（非/users路径）
func TestUserGetAPI_GET008(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caseID := "GET-008"
	caseName := "访问不存在的路由（非/users路径）"
	reqURL := fmt.Sprintf("%s/%s/%s", BASE_URL, API_VERSION, INVALID_ROUTE)
	token := VALID_TOKEN
	expectedStatus := http.StatusMethodNotAllowed
	expectedCode := ERR_METHOD_NOT_ALLOWED

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

// 执行所有用例的入口
func TestUserGetAPI_All(t *testing.T) {
	// 重置统计变量
	total = 0
	passed = 0
	failed = 0
	results = []string{}

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
