package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

// ==================== 核心配置（必须根据真实服务器修改！） ====================
const (
	ServerBaseURL  = "http://localhost:8080" // 你的服务器地址（如192.168.1.100:8080）
	LoginAPIPath   = "/login"                // 登录接口路径
	RequestTimeout = 10 * time.Second        // 请求超时
)

// ==================== 颜色配置（修复Unset报错，用正确的重置方式） ====================
func initColor(c *color.Color) *color.Color {
	c.EnableColor()
	color.NoColor = false // 强制开启颜色
	return c
}

var (
	colorFail    = initColor(color.New(color.FgRed).Add(color.Bold))     // 失败：红色加粗
	colorPass    = initColor(color.New(color.FgGreen).Add(color.Bold))   // 成功：绿色加粗
	colorInfo    = initColor(color.New(color.FgCyan))                    // 信息：青色
	colorCase    = initColor(color.New(color.FgYellow).Add(color.Bold))  // 用例名：黄色加粗
	colorCode    = initColor(color.New(color.FgMagenta).Add(color.Bold)) // 业务码：紫色加粗
	colorCode400 = initColor(color.New(color.FgYellow).Add(color.Bold))  // 400码：黄色加粗
	colorReset   = color.New(color.Reset)                                // 重置颜色（用Print()触发）
)

// ==================== 错误码库（与服务器一致） ====================
type ErrorCode struct {
	Code         int
	ConstantName string
	HTTPStatus   int
	Description  string
}

var errorCodeLibrary = map[string][]ErrorCode{
	"通用基本错误（1000xx）": {
		{100001, "ErrSuccess", 200, "成功"},
		{100004, "ErrValidation", 422, "请求数据语义校验失败"},
		{100007, "ErrUnsupportedMediaType", 415, "不支持的Content-Type"},
	},
	"通用授权认证错误（1002xx）": {
		{100206, "ErrPasswordIncorrect", 401, "密码不正确"},
		{100209, "ErrBase64DecodeFail", 400, "Basic认证Base64解码失败"},
	},
	"iam-apiserver 用户模块（1100xx）": {
		{110001, "ErrUserNotFound", 404, "用户不存在"},
	},
}

// ==================== 通用工具函数 ====================
// sendRealRequest：发送真实HTTP请求
func sendRealRequest(method, url string, headers map[string]string, body string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("创建请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: RequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("服务器无响应: %w", err)
	}

	defer resp.Body.Close()
	respBody := make([]byte, 1024*1024)
	n, err := resp.Body.Read(respBody)
	if err != nil && err.Error() != "EOF" {
		return resp, nil, fmt.Errorf("读取响应失败: %w", err)
	}
	return resp, respBody[:n], nil
}

// getHTTPStatusForCode：根据业务码查HTTP状态码
func getHTTPStatusForCode(code int) int {
	for _, category := range errorCodeLibrary {
		for _, ec := range category {
			if ec.Code == code {
				return ec.HTTPStatus
			}
		}
	}
	return 0
}

// min：避免字符串截取越界
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== 单个用例参数定义 ====================
type SingleTestCaseParams struct {
	Name           string                             // 用例名称
	Headers        map[string]string                  // 请求头
	Body           string                             // 请求体
	ExpectedStatus int                                // 预期HTTP状态码
	ExpectedCode   int                                // 预期业务码
	VerifyData     func(map[string]interface{}) error // Data校验（可选）
}

// ==================== 核心：单个用例执行函数（修复颜色重置报错） ====================
func runSingleTestCase(t *testing.T, params SingleTestCaseParams) {
	t.Helper()
	loginFullURL := ServerBaseURL + LoginAPIPath

	// 1. 打印用例基础信息
	colorCase.Printf("🔍 当前执行用例：%s\n", params.Name)
	colorInfo.Println("----------------------------------------")
	colorInfo.Printf("请求地址: %s\n", loginFullURL)
	colorInfo.Printf("预期结果: HTTP=%d + 业务码=%d\n", params.ExpectedStatus, params.ExpectedCode)
	colorInfo.Println("----------------------------------------")
	colorReset.Print("") // 重置颜色（替代之前的Unset()）

	// 2. 发送请求（失败处理）
	resp, respBody, err := sendRealRequest(http.MethodPost, loginFullURL, params.Headers, params.Body)
	if err != nil {
		colorFail.Println("==================================== 用例失败！====================================")
		colorFail.Printf("❌ 核心原因：请求发送失败（服务器未启动/地址错误）\n")
		colorFail.Printf("   具体错误：%v\n", err)
		colorFail.Printf("   检查项：1. 服务器是否启动  2. ServerBaseURL是否正确\n")
		colorFail.Println("==================================================================================")
		colorReset.Print("") // 重置颜色
		t.Fatal()
	}

	// 3. 解析响应体（重复JSON处理）
	var respBodyMap map[string]interface{}
	parseErr := json.Unmarshal(respBody, &respBodyMap)
	if parseErr != nil {
		colorFail.Println("==================================== 用例失败！====================================")
		colorFail.Printf("❌ 核心原因：服务器返回非法重复JSON，无法解析\n")
		colorFail.Printf("   解析错误：%v\n", parseErr)
		colorFail.Printf("   服务器返回（前500字符）：%s\n", string(respBody[:min(len(respBody), 500)]))
		colorFail.Printf("   修复服务器：参数错误时加return终止，不要返回Token\n")
		colorFail.Println("==================================================================================")
		colorReset.Print("") // 重置颜色
		t.Fatal()
	}

	// 4. 提取响应信息
	actualCode, codeOk := respBodyMap["code"].(float64)
	actualMsg, msgOk := respBodyMap["message"].(string)
	actualHTTPStatus := resp.StatusCode

	// 5. 打印响应信息
	colorInfo.Println("📝 真实响应：")
	colorInfo.Printf("   HTTP状态码：%d\n", actualHTTPStatus)
	if codeOk {
		if int(actualCode) == 400 || getHTTPStatusForCode(int(actualCode)) == 400 {
			colorCode400.Printf("   业务码：%d\n", int(actualCode))
		} else {
			colorCode.Printf("   业务码：%d\n", int(actualCode))
		}
	} else {
		colorFail.Printf("   业务码：未知（无code字段）\n")
	}
	if msgOk {
		colorInfo.Printf("   提示信息：%s\n", actualMsg)
	} else {
		colorInfo.Printf("   提示信息：无\n")
	}
	colorInfo.Println("----------------------------------------")
	colorReset.Print("") // 重置颜色

	// 6. 校验逻辑
	verifyFailed := false
	// 6.1 校验HTTP状态码
	if actualHTTPStatus != params.ExpectedStatus {
		colorFail.Printf("❌ HTTP状态码不匹配：实际=%d，预期=%d\n", actualHTTPStatus, params.ExpectedStatus)
		verifyFailed = true
	} else {
		colorPass.Printf("✅ HTTP状态码匹配：%d\n", actualHTTPStatus)
	}
	// 6.2 校验业务码
	if !codeOk {
		colorFail.Println("❌ 响应缺少code字段")
		verifyFailed = true
	} else if int(actualCode) != params.ExpectedCode {
		colorFail.Printf("❌ 业务码不匹配：实际=%d，预期=%d\n", int(actualCode), params.ExpectedCode)
		verifyFailed = true
	} else {
		colorPass.Printf("✅ 业务码匹配：%d\n", int(actualCode))
	}
	// 6.3 校验Data（可选）
	if params.VerifyData != nil {
		data, dataOk := respBodyMap["data"].(map[string]interface{})
		if !dataOk {
			colorFail.Printf("❌ data格式错误：实际类型=%T\n", respBodyMap["data"])
			verifyFailed = true
		} else if err := params.VerifyData(data); err != nil {
			colorFail.Printf("❌ data校验失败：%v\n", err)
			verifyFailed = true
		} else {
			colorPass.Println("✅ Data校验通过（Token正常）")
		}
	}
	colorReset.Print("") // 重置颜色

	// 7. 最终结论
	if verifyFailed {
		colorFail.Println("==================================== 用例失败！====================================")
		colorFail.Printf("❌ 用例「%s」失败，原因见上方校验\n", params.Name)
		colorFail.Println("==================================================================================")
		colorReset.Print("")
		t.Fail()
	} else {
		colorPass.Println("==================================== 用例成功！====================================")
		colorPass.Printf("✅ 用例「%s」所有校验通过！\n", params.Name)
		colorPass.Println("==================================================================================")
		colorReset.Print("")
	}
}

// ==================== 独立测试用例（支持单个执行） ====================
// TestLogin_InvalidCharAt：用户名含@（你之前失败的用例）
func TestLogin_InvalidCharAt(t *testing.T) {
	params := SingleTestCaseParams{
		Name:           "用户名含非法字符@",
		Headers:        map[string]string{"Content-Type": "application/json"},
		Body:           `{"username":"invalid@user","password":"Admin@2021"}`,
		ExpectedStatus: http.StatusUnprocessableEntity,
		ExpectedCode:   100004,
	}
	runSingleTestCase(t, params)
}

// TestLogin_EmptyUsername：用户名为空
func TestLogin_EmptyUsername(t *testing.T) {
	params := SingleTestCaseParams{
		Name:           "用户名为空",
		Headers:        map[string]string{"Content-Type": "application/json"},
		Body:           `{"username":"","password":"Admin@2021"}`,
		ExpectedStatus: http.StatusUnprocessableEntity,
		ExpectedCode:   100004,
	}
	runSingleTestCase(t, params)
}

// TestLogin_SuccessWithToken：登录成功（需真实账号密码）
func TestLogin_SuccessWithToken(t *testing.T) {
	verifyToken := func(data map[string]interface{}) error {
		token, ok := data["token"].(string)
		if !ok || token == "" || strings.Count(token, ".") != 2 {
			return fmt.Errorf("Token无效：%v", token)
		}
		return nil
	}

	params := SingleTestCaseParams{
		Name:           "登录成功（返回Token）",
		Headers:        map[string]string{"Content-Type": "application/json"},
		Body:           `{"username":"admin","password":"Admin@2021"}`, // 替换为真实账号
		ExpectedStatus: http.StatusOK,
		ExpectedCode:   100001,
		VerifyData:     verifyToken,
	}
	runSingleTestCase(t, params)
}

// ==================== 批量执行入口 ====================
func TestLogin_AllCases(t *testing.T) {
	colorInfo.Println(strings.Repeat("=", 80))
	colorInfo.Println("🚀 开始执行所有登录接口测试用例")
	colorInfo.Printf("   服务器：%s\n", ServerBaseURL)
	colorInfo.Printf("   超时：%v\n", RequestTimeout)
	colorInfo.Println(strings.Repeat("=", 80) + "\n")
	colorReset.Print("")

	// 执行所有用例
	t.Run("用例1：用户名含@", TestLogin_InvalidCharAt)
	t.Run("用例2：用户名为空", TestLogin_EmptyUsername)
	t.Run("用例3：登录成功", TestLogin_SuccessWithToken)

	colorInfo.Println(strings.Repeat("=", 80))
	colorInfo.Println("🏁 所有用例执行完毕！")
	colorInfo.Println(strings.Repeat("=", 80))
	colorReset.Print("")
}
