package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------- 全局配置（根据实际接口修改） --------------------------
const (
	TestBaseURL        = "http://localhost:8080" // 接口基础地址
	TestLoginPath      = "/login"                // 登录接口路径
	ValidUsername      = "admin"                 // 有效用户名
	ValidPassword      = "Admin@2021"            // 有效密码
	InvalidUser        = "invalid@user"          // 含非法字符的用户名
	WeakPassword       = "weak"                  // 弱密码
	ExpectedSuccCode   = 0                       // 成功业务码（根据实际返回修改）
	ExpectedValiCode   = 100001                  // 参数校验错误码
	ExpectedPwdErrCode = 110003                  // 密码错误业务码（根据实际返回修改）
)

// -------------------------- 彩色输出工具 --------------------------
var (
	colorInfo      = color.New(color.FgBlue).SprintFunc()
	colorSuccess   = color.New(color.FgGreen).SprintFunc()
	colorError     = color.New(color.FgRed).SprintFunc()
	colorBold      = color.New(color.Bold).SprintFunc()
	colorUnderline = color.New(color.Underline).SprintFunc()
)

// -------------------------- 通用结构体定义 --------------------------
// CommonAPIResponse 通用响应结构体，适配大多数API的code/message/data格式
type CommonAPIResponse struct {
	Code    interface{} `json:"code"`    // 业务码（支持int/string类型）
	Message string      `json:"message"` // 提示信息
	Data    interface{} `json:"data"`    // 业务数据
}

// LoginReq 登录请求体结构
type LoginReq struct {
	Username string `json:"username"` // 用户名（字段名根据接口修改）
	Password string `json:"password"` // 密码（字段名根据接口修改）
}

// -------------------------- 核心测试函数 --------------------------
func TestLoginRESTCompliance(t *testing.T) {
	// 测试初始化提示
	fmt.Printf("\n%s\n", colorBold(colorUnderline("=== 登录接口 RESTful 合规性测试 ===")))
	fmt.Printf("%s 测试目标：%s%s\n", colorInfo("[INFO]"), colorUnderline(TestBaseURL), TestLoginPath)
	fmt.Printf("%s 开始执行测试用例...\n", colorInfo("[INFO]"))

	// 定义测试用例
	testCases := []struct {
		name            string
		httpMethod      string
		contentType     string
		reqBody         interface{}
		expectedHTTP    int
		expectedBizCode interface{}
		verifyData      func(t *testing.T, data interface{})
	}{
		// 用例1：正常登录
		{
			name:            "正常登录",
			httpMethod:      http.MethodPost,
			contentType:     "application/json",
			reqBody:         LoginReq{Username: ValidUsername, Password: ValidPassword},
			expectedHTTP:    http.StatusOK,
			expectedBizCode: ExpectedSuccCode,
			verifyData: func(t *testing.T, data interface{}) {
				dataMap, ok := data.(map[string]interface{})
				require.True(t, ok, colorError("响应Data不是JSON对象"))

				// 验证token
				token, hasToken := dataMap["token"].(string)
				require.True(t, hasToken, colorError("响应缺少token字段"))
				assert.NotEmpty(t, token, colorError("token为空"))
				fmt.Printf("%s 令牌有效（长度：%d）\n", colorInfo("[DEBUG]"), len(token))

				// 验证过期时间
				expireStr, hasExpire := dataMap["expire"].(string)
				require.True(t, hasExpire, colorError("响应缺少expire字段"))
				_, err := time.Parse(time.RFC3339, expireStr)
				require.NoError(t, err, colorError("expire格式错误（需RFC3339）：%s", expireStr))
			},
		},

		// 用例2：拒绝GET方法
		{
			name:            "拒绝GET方法",
			httpMethod:      http.MethodGet,
			contentType:     "application/json",
			reqBody:         nil,
			expectedHTTP:    http.StatusMethodNotAllowed,
			expectedBizCode: 100003, // 方法不允许业务码
			verifyData:      nil,
		},

		// 用例3：拒绝表单格式
		{
			name:            "拒绝表单格式",
			httpMethod:      http.MethodPost,
			contentType:     "application/x-www-form-urlencoded",
			reqBody:         fmt.Sprintf("username=%s&password=%s", ValidUsername, ValidPassword),
			expectedHTTP:    http.StatusUnsupportedMediaType,
			expectedBizCode: ExpectedValiCode,
			verifyData:      nil,
		},

		// 用例4：用户名含非法字符
		{
			name:            "用户名含非法字符",
			httpMethod:      http.MethodPost,
			contentType:     "application/json",
			reqBody:         LoginReq{Username: InvalidUser, Password: ValidPassword},
			expectedHTTP:    http.StatusBadRequest,
			expectedBizCode: ExpectedValiCode,
			verifyData:      nil,
		},

		// 用例5：密码强度不足
		{
			name:            "密码强度不足",
			httpMethod:      http.MethodPost,
			contentType:     "application/json",
			reqBody:         LoginReq{Username: ValidUsername, Password: WeakPassword},
			expectedHTTP:    http.StatusBadRequest,
			expectedBizCode: ExpectedValiCode,
			verifyData:      nil,
		},

		// 用例6：密码错误
		{
			name:            "密码错误",
			httpMethod:      http.MethodPost,
			contentType:     "application/json",
			reqBody:         LoginReq{Username: ValidUsername, Password: "WrongPass123!"},
			expectedHTTP:    http.StatusUnauthorized,
			expectedBizCode: ExpectedPwdErrCode,
			verifyData:      nil,
		},
	}

	// 执行测试用例
	for idx, tc := range testCases {
		caseIdx := idx + 1
		t.Run(tc.name, func(t *testing.T) {
			// 构造请求体
			var reqBodyBytes []byte
			if tc.reqBody != nil {
				if tc.contentType == "application/json" {
					var err error
					reqBodyBytes, err = json.Marshal(tc.reqBody)
					require.NoError(t, err, colorError("序列化请求体失败：%v", err))
				} else {
					reqBodyBytes = []byte(fmt.Sprintf("%v", tc.reqBody))
				}
			}

			// 创建HTTP请求
			fullURL := fmt.Sprintf("%s%s", TestBaseURL, TestLoginPath)
			req, err := http.NewRequest(tc.httpMethod, fullURL, bytes.NewBuffer(reqBodyBytes))
			require.NoError(t, err, colorError("创建请求失败：%v", err))
			req.Header.Set("Content-Type", tc.contentType)

			// 发送请求
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err, colorError("发送请求失败：%v（URL：%s）", err, fullURL))
			defer resp.Body.Close()

			// 断言HTTP状态码
			assert.Equal(t, tc.expectedHTTP, resp.StatusCode,
				colorError("HTTP状态码不匹配：实际 %d，预期 %d", resp.StatusCode, tc.expectedHTTP))
			if resp.StatusCode == tc.expectedHTTP {
				fmt.Printf("%s 用例%d：HTTP状态码 %d（符合预期）\n", colorSuccess("[PASS]"), caseIdx, resp.StatusCode)
			} else {
				fmt.Printf("%s 用例%d：HTTP状态码 %d（预期 %d）\n", colorError("[FAIL]"), caseIdx, resp.StatusCode, tc.expectedHTTP)
				return
			}

			// 解析响应体
			var apiResp CommonAPIResponse
			err = json.NewDecoder(resp.Body).Decode(&apiResp)
			require.NoError(t, err, colorError("解析响应体失败（非JSON格式）"))

			// 断言业务码（修复类型匹配问题）
			actualCodeStr := fmt.Sprintf("%v", apiResp.Code)         // 实际码转字符串
			expectedCodeStr := fmt.Sprintf("%v", tc.expectedBizCode) // 预期码转字符串
			bizCodeMatch := (actualCodeStr == expectedCodeStr)

			assert.True(t, bizCodeMatch,
				colorError("业务码不匹配：实际 %v（类型：%T），预期 %v（类型：%T）",
					apiResp.Code, apiResp.Code,
					tc.expectedBizCode, tc.expectedBizCode))

			if bizCodeMatch {
				fmt.Printf("%s 用例%d：业务码 %v（符合预期）\n", colorSuccess("[PASS]"), caseIdx, apiResp.Code)
			} else {
				fmt.Printf("%s 用例%d：业务码 %v（预期 %v）\n", colorError("[FAIL]"), caseIdx, apiResp.Code, tc.expectedBizCode)
				return
			}

			// 验证业务数据
			if tc.verifyData != nil && bizCodeMatch {
				fmt.Printf("%s 用例%d：验证业务数据...\n", colorInfo("[INFO]"), caseIdx)
				tc.verifyData(t, apiResp.Data)
				fmt.Printf("%s 用例%d：业务数据验证通过\n", colorSuccess("[PASS]"), caseIdx)
			}

			// 用例完成
			fmt.Printf("%s 用例%d：%s（执行完成）\n", colorSuccess("[SUCCESS]"), caseIdx, tc.name)
		})
	}

	// 测试总结
	fmt.Printf("\n%s\n", colorBold(colorSuccess("=== 所有测试用例执行完成！ ===")))
}

// 主函数：支持直接运行
func main() {
	testing.Main(
		func(pat, str string) (bool, error) { return true, nil },
		[]testing.InternalTest{{Name: "TestLoginRESTCompliance", F: TestLoginRESTCompliance}},
		nil,
		nil,
	)
}
