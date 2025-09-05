package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
)

// 颜色定义
var (
	colorPass    = color.New(color.FgGreen).Add(color.Bold)
	colorFail    = color.New(color.FgRed).Add(color.Bold)
	colorInfo    = color.New(color.FgCyan)
	colorCase    = color.New(color.FgYellow).Add(color.Bold)
	colorCode    = color.New(color.FgMagenta).Add(color.Bold)
	colorCode400 = color.New(color.FgYellow).Add(color.Bold) // 400相关错误码颜色
	colorReset   = color.New(color.Reset)
)

// 错误码结构体定义
type ErrorCode struct {
	Code         int
	ConstantName string
	HTTPStatus   int
	Description  string
}

// 错误码库 - 实际业务中使用的完整错误码集合
var errorCodeLibrary = map[string][]ErrorCode{
	"基本错误": {
		{1, "未知常量名", 500, "发生了内部服务器错误,请参阅http://git..."},
	},
	"通用基本错误（1000xx）": {
		{100001, "ErrSuccess", 200, "成功"},
		{100002, "ErrUnknown", 500, "内部服务器错误"},
		{100003, "ErrBind", 400, "请求体绑定结构体失败"},
		{100004, "ErrValidation", 422, "请求数据语义校验失败"},
		{100005, "ErrPageNotFound", 404, "页面不存在"},
		{100006, "ErrMethodNotAllowed", 405, "方法不允许"},
		{100007, "ErrUnsupportedMediaType", 415, "不支持的Content-Type，仅支持application/json"},
	},
	"通用数据库错误（1001xx）": {
		{100101, "ErrDatabase", 500, "数据库操作错误"},
		{100102, "ErrDatabaseTimeout", 504, "数据库操作超时"},
	},
	"通用授权认证错误（1002xx）": {
		{100201, "ErrEncrypt", 500, "用户密码加密失败"},
		{100202, "ErrSignatureInvalid", 401, "签名无效"},
		{100203, "ErrExpired", 401, "令牌已过期"},
		{100204, "ErrInvalidAuthHeader", 400, "授权头格式无效"},
		{100205, "ErrMissingHeader", 401, "缺少 Authorization 头"},
		{100206, "ErrPasswordIncorrect", 401, "密码不正确"},
		{100207, "ErrPermissionDenied", 403, "权限不足，无操作权限"},
		{100208, "ErrTokenInvalid", 401, "令牌无效（格式/签名错误）"},
		{100209, "ErrBase64DecodeFail", 400, "Basic认证 payload Base64解码失败（请确保正确编码）"},
		{100210, "ErrInvalidBasicPayload", 400, "Basic认证认证 payload格式无效（需用冒号分隔）"},
	},
	"通用加解码错误（1003xx）": {
		{100301, "ErrEncodingFailed", 500, "数据编码失败"},
		{100302, "ErrDecodingFailed", 400, "数据解码失败（格式错误）"},
		{100303, "ErrInvalidJSON", 400, "数据不是有效的 JSON 格式"},
		{100304, "ErrEncodingJSON", 500, "JSON 数据编码失败"},
		{100305, "ErrDecodingJSON", 400, "JSON 数据解码失败（格式错误）"},
		{100306, "ErrInvalidYaml", 400, "数据不是有效的 YAML 格式"},
		{100307, "ErrEncodingYaml", 500, "YAML 数据编码失败"},
		{100308, "ErrDecodingYaml", 400, "YAML 数据解码失败（格式错误）"},
	},
	"iam-apiserver 用户模块（1100xx）": {
		{110001, "ErrUserNotFound", 404, "用户不存在"},
		{110002, "ErrUserAlreadyExist", 409, "用户已存在（用户名冲突）"},
		{110003, "ErrUnauthorized", 401, "未授权访问用户资源"},
		{110004, "ErrInvalidParameter", 400, "用户参数无效（如用户名为空）"},
		{110005, "ErrInternal", 500, "用户模块内部逻辑错误"},
		{110006, "ErrResourceConflict", 409, "用户资源冲突（如角色已绑定）"},
		{110007, "ErrInternalServer", 500, "用户模块服务器内部错误"},
	},
	"iam-apiserver 密钥模块（1101xx）": {
		{110101, "ErrReachMaxCount", 400, "密钥数量达到上限（最多支持 10 个）"},
		{110102, "ErrSecretNotFound", 404, "密钥不存在"},
	},
	"iam-apiserver 策略模块（1102xx）": {
		{110201, "ErrPolicyNotFound", 404, "策略不存在"},
	},
}

// 打印错误码库信息
func printErrorCodeLibrary() {
	colorInfo.Println("\n" + strings.Repeat("=", 100))
	colorInfo.Println("业务错误码库信息")
	colorInfo.Println(strings.Repeat("=", 100))

	for category, codes := range errorCodeLibrary {
		colorCase.Printf("\n===== %s =====\n", category)
		fmt.Printf("%-10s %-20s %-10s %s\n",
			"错误码", "常量名", "HTTP状态", "描述信息")
		fmt.Println(strings.Repeat("-", 100))

		for _, code := range codes {
			// 400相关错误码用特殊颜色显示
			if code.HTTPStatus == 400 {
				colorCode400.Printf("%-10d ", code.Code)
			} else {
				colorCode.Printf("%-10d ", code.Code)
			}

			fmt.Printf("%-20s ", code.ConstantName)
			colorCode.Printf("%-10d ", code.HTTPStatus)
			colorReset.Print(code.Description + "\n")
		}
	}

	colorInfo.Println("\n" + strings.Repeat("=", 100) + "\n")
	color.Unset()
}

// 路由处理逻辑
func setupTestRouter() *gin.Engine {
	r := gin.Default()
	r.POST("/login", func(c *gin.Context) {
		// 检查Content-Type
		contentType := c.Request.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"code":    100007,
				"message": "不支持的Content-Type，仅支持application/json",
			})
			return
		}

		// 处理Basic认证
		authHeader := c.Request.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Basic ") {
			encoded := strings.TrimPrefix(authHeader, "Basic ")
			_, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    100209,
					"message": "Basic认证 payload Base64解码失败（请确保正确编码）",
				})
				return
			}
		}

		// 解析请求体
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    100004,
				"message": "请求数据语义校验失败",
			})
			return
		}

		// 参数校验
		if req.Username == "" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    100004,
				"message": "请求数据语义校验失败: 用户名为空",
			})
			return
		}
		if strings.Contains(req.Username, "@") {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    100004,
				"message": "请求数据语义校验失败: 用户名含非法字符@",
			})
			return
		}
		if len(req.Password) < 6 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    100004,
				"message": "请求数据语义校验失败: 密码过短",
			})
			return
		}

		// 认证逻辑
		switch {
		case req.Username == "notexist":
			c.JSON(http.StatusNotFound, gin.H{
				"code":    110001,
				"message": "用户不存在",
			})
		case req.Username == "validuser" && req.Password != "Valid@2021":
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    100206,
				"message": "密码不正确",
			})
		case req.Username == "validuser" && req.Password == "Valid@2021":
			c.JSON(http.StatusOK, gin.H{
				"code":    100001,
				"message": "登录成功",
				"data":    map[string]string{"token": "test-token-123"},
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    100002,
				"message": "内部服务器错误",
			})
		}
	})
	return r
}

// 打印测试结果统计
func printSummary(total, passed, failed int) {
	colorInfo.Println("\n" + strings.Repeat("=", 80))
	colorInfo.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d\n", total, passed, failed)
	colorInfo.Println(strings.Repeat("=", 80))

	if failed == 0 {
		colorPass.Println("🎉 所有测试用例全部通过!")
	} else {
		colorFail.Printf("❌ 有 %d 个测试用例失败，请检查问题\n", failed)
	}
	color.Unset()
}

// 测试用例执行
func TestLoginAllCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()
	var total, passed, failed int

	// 首先打印错误码库信息
	printErrorCodeLibrary()

	testCases := []struct {
		name           string
		method         string
		headers        map[string]string
		body           string
		expectedStatus int
		expectedCode   int
		verifyData     func(map[string]interface{}) error
	}{
		// 参数校验场景
		{
			name:           "用户名含非法字符@",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"invalid@user","password":"Admin@2021"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   100004,
		},
		{
			name:           "用户名为空",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"","password":"Admin@2021"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   100004,
		},
		{
			name:           "密码过短（仅3位）",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"validuser","password":"123"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   100004,
		},
		{
			name:           "JSON格式错误（缺少引号）",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"validuser",password:"Valid@2021"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   100004,
		},
		{
			name:           "缺少password字段",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"validuser"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   100004,
		},
		// 认证逻辑场景
		{
			name:           "用户不存在",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"notexist","password":"AnyPass@2021"}`,
			expectedStatus: http.StatusNotFound,
			expectedCode:   110001,
		},
		{
			name:           "密码不正确",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"validuser","password":"Wrong@2021"}`,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   100206,
		},
		// 成功场景
		{
			name:           "登录成功（返回token）",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"username":"validuser","password":"Valid@2021"}`,
			expectedStatus: http.StatusOK,
			expectedCode:   100001,
			verifyData: func(data map[string]interface{}) error {
				if _, ok := data["token"].(string); !ok {
					return fmt.Errorf("token缺失")
				}
				return nil
			},
		},
		// 其他格式场景
		{
			name:           "不支持的Content-Type（表单）",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			body:           "username=validuser&password=Valid@2021",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedCode:   100007,
		},
		{
			name:           "Basic认证格式错误（无效token）",
			method:         http.MethodPost,
			headers:        map[string]string{"Content-Type": "application/json", "Authorization": "Basic invalid-base64-token"},
			body:           `{"username":"validuser","password":"Valid@2021"}`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   100209,
		},
	}

	// 打印测试开始信息
	colorInfo.Println(strings.Repeat("=", 80))
	colorInfo.Println("开始执行登录接口测试用例")
	colorInfo.Println(strings.Repeat("=", 80) + "\n")

	// 执行测试用例
	for idx, tc := range testCases {
		total++
		caseIdx := idx + 1

		// 打印用例标题
		colorCase.Printf("用例 %d: %s\n", caseIdx, tc.name)
		colorInfo.Println("----------------------------------------")

		// 创建请求
		req := httptest.NewRequest(tc.method, "/login", strings.NewReader(tc.body))
		for k, v := range tc.headers {
			req.Header.Set(k, v)
		}

		// 发送请求
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		resp := w.Result()
		defer resp.Body.Close()

		// 解析响应体
		var respBody map[string]interface{}
		var parseErr error
		if parseErr = json.NewDecoder(resp.Body).Decode(&respBody); parseErr != nil {
			colorFail.Printf("❌ 响应解析失败: %v\n", parseErr)
		}

		// 打印实际返回的业务码和消息
		if parseErr == nil {
			actualCode, codeOk := respBody["code"].(float64)
			message, msgOk := respBody["message"].(string)

			colorInfo.Print("实际返回: ")
			if codeOk {
				if int(actualCode) == 400 || getHTTPStatusForCode(int(actualCode)) == 400 {
					colorCode400.Printf("code=%d ", int(actualCode))
				} else {
					colorCode.Printf("code=%d ", int(actualCode))
				}
			} else {
				colorFail.Print("code=未知 ")
			}

			if msgOk {
				fmt.Printf("message=%s\n", message)
			} else {
				colorFail.Print("message=未知\n")
			}
		}

		// 校验状态码
		casePassed := true
		if resp.StatusCode != tc.expectedStatus {
			colorFail.Printf("❌ 状态码错误: 预期 %d, 实际 %d\n",
				tc.expectedStatus, resp.StatusCode)
			casePassed = false
		} else {
			colorInfo.Printf("✅ 状态码正确: %d\n", resp.StatusCode)
		}

		// 校验业务码
		if parseErr == nil {
			actualCode, ok := respBody["code"].(float64)
			if !ok {
				colorFail.Println("❌ 响应缺少code字段")
				casePassed = false
			} else if int(actualCode) != tc.expectedCode {
				colorFail.Printf("❌ 业务码错误: 预期 %d, 实际 %d\n",
					tc.expectedCode, int(actualCode))
				casePassed = false
			} else {
				colorCode.Printf("✅ 业务码正确: %d\n", int(actualCode))
			}
		} else {
			casePassed = false
		}

		// 校验成功场景数据
		if tc.verifyData != nil && casePassed && parseErr == nil {
			if data, ok := respBody["data"].(map[string]interface{}); ok {
				if err := tc.verifyData(data); err != nil {
					colorFail.Printf("❌ 数据校验失败: %v\n", err)
					casePassed = false
				} else {
					colorInfo.Println("✅ 响应数据校验通过")
				}
			} else {
				colorFail.Println("❌ data字段格式错误")
				casePassed = false
			}
		}

		// 统计结果
		if casePassed {
			passed++
			colorPass.Println("----------------------------------------")
			colorPass.Println("用例执行通过 ✅\n")
		} else {
			failed++
			colorFail.Println("----------------------------------------")
			colorFail.Println("用例执行失败 ❌\n")
		}
		color.Unset()
	}

	// 打印总结
	printSummary(total, passed, failed)
}

// 根据业务码获取对应的HTTP状态码（用于颜色显示）
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

// 支持go run执行
func main() {
	testing.Main(
		func(pat, str string) (bool, error) { return true, nil },
		[]testing.InternalTest{{Name: "TestLoginAllCases", F: TestLoginAllCases}},
		nil, nil,
	)
}
