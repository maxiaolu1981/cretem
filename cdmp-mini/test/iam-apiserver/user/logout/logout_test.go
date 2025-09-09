package logout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// -------------------------- 终端颜色控制常量 --------------------------
const (
	// 基础颜色
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"

	// 高亮颜色
	colorBrightRed    = "\033[91m"
	colorBrightGreen  = "\033[92m"
	colorBrightYellow = "\033[93m"
	colorBrightBlue   = "\033[94m"
	colorBrightPurple = "\033[95m"
	colorBrightCyan   = "\033[96m"

	// 样式控制
	colorBold  = "\033[1m"
	colorReset = "\033[0m" // 重置所有样式
)

// 检查终端是否支持颜色输出
var supportsColor = func() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}()

// 颜色包装函数 - 仅接受两个参数：文本和颜色代码
func withColor(text, color string) string {
	if supportsColor {
		return color + text + colorReset
	}
	return text
}

// 打印分隔线
func printSeparator() {
	fmt.Println(withColor(strings.Repeat("-", 80), colorCyan))
}

// -------------------------- 基础定义 --------------------------
const (
	ErrMissingHeader     = 100205                         // 无令牌
	ErrInvalidAuthHeader = 100204                         // 授权头格式错误
	ErrTokenInvalid      = 100208                         // 令牌格式错误
	ErrExpired           = 100203                         // 令牌过期
	ErrSignatureInvalid  = 100202                         // 签名无效
	ErrUnauthorized      = 110003                         // 未授权
	ErrInternal          = 50001                          // 服务器内部错误
	SuccessCode          = 100001                         // 成功业务码
	RealServerURL        = "http://localhost:8080/logout" // 真实服务器地址
	LoginURL             = "http://localhost:8080/login"  // 登录接口地址
)

type withCodeError struct {
	code    int
	message string
}

func (e *withCodeError) Error() string { return e.message }
func withCode(code int, msg string) error {
	return &withCodeError{code: code, message: msg}
}
func getErrCode(err error) int {
	if e, ok := err.(*withCodeError); ok {
		return e.code
	}
	return ErrUnauthorized
}
func getErrMsg(err error) string {
	if e, ok := err.(*withCodeError); ok {
		return e.message
	}
	return "unknown error"
}

// 修复：添加Role字段，与真实Token结构一致
type CustomClaims struct {
	UserID   string `json:"user_id"`
	Role     string `json:"role"`     // 新增：角色字段
	Username string `json:"username"` // 新增：用户名字段
	jwt.RegisteredClaims
}

var (
	localTokenBlacklist = make(map[string]bool)
	mu                  sync.Mutex
)

func markTokenAsLoggedOut(tokenString string) {
	mu.Lock()
	defer mu.Unlock()
	localTokenBlacklist[trimBearerPrefix(tokenString)] = true
}
func isLocalTokenLoggedOut(tokenString string) bool {
	mu.Lock()
	defer mu.Unlock()
	return localTokenBlacklist[trimBearerPrefix(tokenString)]
}
func trimBearerPrefix(token string) string {
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return strings.TrimPrefix(token, "Bearer ")
	}
	return token
}

// -------------------------- 测试工具函数 --------------------------

// 从登录接口获取真实Token（推荐方式）
func getTokenFromLogin(httpClient *http.Client, username, password string) (string, error) {
	// 构造登录请求
	loginData := map[string]string{
		"username": username,
		"password": password,
	}
	data, err := json.Marshal(loginData)
	if err != nil {
		return "", fmt.Errorf("构造登录请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", LoginURL, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("创建登录请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("登录失败，状态码: %d", resp.StatusCode)
	}

	// 解析登录响应
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v", err)
	}

	token, ok := result["token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("登录响应中未找到有效token")
	}

	return token, nil
}

// 生成测试用Token（包含role字段）
func generateTestToken(isExpired, isInvalidSign bool) (string, error) {
	jwtSecret := []byte(viper.GetString("jwt.key"))
	invalidSecret := []byte("wrong-secret-654321")

	// 修复：添加role字段，值为"admin"，与真实登录生成的Token一致
	claims := &CustomClaims{
		UserID:   "test-user-123",
		Username: "admin",
		Role:     "admin", // 关键修复：添加角色信息
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    viper.GetString("jwt.issuer"),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	if isExpired {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Minute))
	}

	secret := jwtSecret
	if isInvalidSign {
		secret = invalidSecret
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("生成 Token 失败: %v", err)
	}
	return signedToken, nil
}

func maskToken(token string) string {
	if len(token) <= 20 {
		return token
	}
	return token[:20] + "..."
}

// -------------------------- 测试用例定义 --------------------------
type LogoutTestSuite struct {
	CaseID         string // 用例编号（LOGOUT-001）
	Desc           string // 用例描述
	AuthHeader     string // Authorization请求头（带Bearer前缀）
	ExpectedStatus int    // 预期HTTP状态码
	ExpectedCode   int    // 预期业务码
	ExpectedMsg    string // 预期消息（包含匹配）
	Passed         bool   // 执行结果
}

// -------------------------- 核心测试函数 --------------------------
func TestLogoutAPI_RealServer(t *testing.T) {
	// 打印测试开始信息
	fmt.Println(withColor("\n==============================================", colorBrightBlue+colorBold))
	fmt.Println(withColor("          登出接口真实服务器测试套件          ", colorBrightBlue+colorBold))
	fmt.Println(withColor("==============================================\n", colorBrightBlue+colorBold))

	// 1. 初始化配置
	viper.SetConfigFile("../../configs/config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		// 若没有配置文件，直接硬编码关键配置
		viper.Set("jwt.key", "dfVpOK8LZeJLZHYmHdb1VdyRrACKpqoo")
		viper.Set("jwt.issuer", "iam-apiserver")
		viper.Set("http.timeout", 5)
	}

	// 2. 清理本地测试数据
	mu.Lock()
	localTokenBlacklist = make(map[string]bool)
	mu.Unlock()

	// 3. 初始化HTTP客户端
	httpClient := &http.Client{
		Timeout: time.Duration(viper.GetInt("http.timeout")) * time.Second,
	}

	// 4. 获取测试用Token（优先从登录接口获取真实Token）
	var normalToken string
	var err error

	// 尝试从登录接口获取真实Token
	normalToken, err = getTokenFromLogin(httpClient, "test-admin", "test-password")
	if err != nil {
		fmt.Println(withColor(fmt.Sprintf("从登录接口获取Token失败，使用备用方式生成: %v", err), colorYellow))
		// 备用方案：生成包含role的测试Token
		normalToken, err = generateTestToken(false, false)
		if err != nil {
			t.Fatalf("%s", withColor(fmt.Sprintf("[初始化失败] 生成正常 Token 失败: %v", err), colorBrightRed))
		}
	}

	// 生成其他测试用Token
	expiredToken, _ := generateTestToken(true, false)
	invalidSignToken, _ := generateTestToken(false, true)
	invalidFormatToken := "Bearer invalid-token-no-dot-123"

	// 5. 定义测试用例
	testSuites := []*LogoutTestSuite{
		{
			CaseID:         "LOGOUT-001",
			Desc:           "无Authorization请求头",
			AuthHeader:     "",
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrMissingHeader,
			ExpectedMsg:    "请先登录",
		},
		{
			CaseID:         "LOGOUT-002",
			Desc:           "Authorization格式无效（仅Bearer前缀，无Token）",
			AuthHeader:     "Bearer ",
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrInvalidAuthHeader,
			ExpectedMsg:    "invalid authorization header format",
		},
		{
			CaseID:         "LOGOUT-003",
			Desc:           "令牌格式错误（无.分隔）",
			AuthHeader:     invalidFormatToken,
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrTokenInvalid,
			ExpectedMsg:    "令牌格式错误",
		},
		{
			CaseID:         "LOGOUT-004",
			Desc:           "令牌已过期",
			AuthHeader:     "Bearer " + expiredToken,
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrExpired,
			ExpectedMsg:    "令牌已过期",
		},
		{
			CaseID:         "LOGOUT-005",
			Desc:           "令牌签名无效",
			AuthHeader:     "Bearer " + invalidSignToken,
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrSignatureInvalid,
			ExpectedMsg:    "signature is invalid",
		},
		{
			CaseID:         "LOGOUT-006",
			Desc:           "正常令牌登出成功",
			AuthHeader:     "Bearer " + normalToken,
			ExpectedStatus: http.StatusOK,
			ExpectedCode:   SuccessCode,
			ExpectedMsg:    "登出成功",
			Passed:         false,
		},
		{
			CaseID:         "LOGOUT-007",
			Desc:           "已登出令牌再次登出（依赖 LOGOUT-006 执行成功）",
			AuthHeader:     "Bearer " + normalToken,
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrUnauthorized,
			ExpectedMsg:    "令牌已登出",
		},
	}

	// 6. 执行测试用例
	var total, passed int
	testResults := make([]*LogoutTestSuite, 0, len(testSuites))

	for _, suite := range testSuites {
		t.Run(suite.CaseID, func(t *testing.T) {
			// 跳过 LOGOUT-007（如果 LOGOUT-006 未通过）
			if suite.CaseID == "LOGOUT-007" {
				if len(testResults) < 6 || !testResults[5].Passed {
					skipMsg := fmt.Sprintf("[%s] %s → 因依赖用例未通过而跳过", suite.CaseID, suite.Desc)
					fmt.Println(withColor(skipMsg, colorBrightYellow))
					printSeparator()
					t.Skip(skipMsg)
					return
				}
			}

			// 用例开始信息
			fmt.Println(withColor(fmt.Sprintf("\n测试用例: %s - %s", suite.CaseID, suite.Desc), colorBrightBlue+colorBold))
			printSeparator()

			// 请求信息
			fmt.Println(withColor("请求信息:", colorCyan+colorBold))
			fmt.Printf("  %-20s %s\n", "请求地址:", withColor(RealServerURL, colorCyan))

			authHeader := maskToken(suite.AuthHeader)
			if authHeader == "" {
				fmt.Printf("  %-20s %s\n", "Authorization头:", withColor("无", colorCyan))
			} else {
				fmt.Printf("  %-20s %s\n", "Authorization头:", withColor(authHeader, colorCyan))
			}

			// 预期结果
			fmt.Println(withColor("\n预期结果:", colorBrightPurple+colorBold))
			fmt.Printf("  %-20s %d\n", "HTTP状态码:", suite.ExpectedStatus)
			fmt.Printf("  %-20s %d\n", "业务码:", suite.ExpectedCode)
			fmt.Printf("  %-20s %s\n", "消息包含:", withColor(suite.ExpectedMsg, colorBrightPurple))

			// 创建并发送请求
			req, err := http.NewRequest("DELETE", RealServerURL, nil)
			if err != nil {
				suite.Passed = false
				errMsg := fmt.Sprintf("创建请求失败: %v", err)
				fmt.Println(withColor("\n请求错误:", colorBrightRed+colorBold))
				fmt.Println(withColor("  "+errMsg, colorBrightRed))
				t.Errorf("%s", errMsg)
				printSeparator()
				return
			}
			fmt.Printf("即将设置authorization头:[%q],长度是%d\n", suite.AuthHeader, len(suite.AuthHeader))

			// 设置请求头
			if suite.AuthHeader != "" {
				req.Header.Set("Authorization", suite.AuthHeader)
			}
			req.Header.Set("Content-Type", "application/json")

			// 发送请求
			resp, err := httpClient.Do(req)
			if err != nil {
				suite.Passed = false
				errMsg := fmt.Sprintf("发送请求失败（可能服务器未启动）: %v", err)
				fmt.Println(withColor("\n请求错误:", colorBrightRed+colorBold))
				fmt.Println(withColor("  "+errMsg, colorBrightRed))
				t.Errorf("%s", errMsg)
				printSeparator()
				return
			}
			defer resp.Body.Close()

			// 解析响应
			var respBody map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
				suite.Passed = false
				errMsg := fmt.Sprintf("解析响应体失败（非JSON格式）: %v", err)
				fmt.Println(withColor("\n响应解析错误:", colorBrightRed+colorBold))
				fmt.Println(withColor("  "+errMsg, colorBrightRed))
				t.Errorf("%s", errMsg)
				printSeparator()
				return
			}

			// 提取响应字段
			actualStatus := resp.StatusCode
			actualCode, _ := respBody["code"].(float64)
			actualMsg, _ := respBody["message"].(string)
			rawResp, _ := json.MarshalIndent(respBody, "", "  ")

			// 显示实际响应
			fmt.Println(withColor("\n实际响应:", colorBlue+colorBold))
			fmt.Printf("  %-20s %d\n", "HTTP状态码:", actualStatus)
			fmt.Printf("  %-20s %d\n", "业务码:", int(actualCode))
			fmt.Printf("  %-20s %s\n", "消息:", actualMsg)
			fmt.Printf("  %-20s %s\n", "原始响应体:", withColor(string(rawResp), colorBlue))

			// 验证结果
			statusPass := assert.Equal(t, suite.ExpectedStatus, actualStatus, "[%s] HTTP状态码不符", suite.CaseID)
			codePass := assert.Equal(t, suite.ExpectedCode, int(actualCode), "[%s] 业务码不符", suite.CaseID)
			msgPass := assert.Contains(t, actualMsg, suite.ExpectedMsg,
				"[%s] 消息不符：预期包含「%s」，实际「%s」", suite.CaseID, suite.ExpectedMsg, actualMsg)

			// 标记结果并显示
			suite.Passed = statusPass && codePass && msgPass
			fmt.Println("\n" + withColor("测试结果:", colorBold))

			if suite.Passed {
				successMsg := fmt.Sprintf("✅ 测试通过 - %s", suite.CaseID)
				fmt.Println(withColor("  "+successMsg, colorBrightGreen))

				// LOGOUT-006 成功后标记Token为已登出
				if suite.CaseID == "LOGOUT-006" {
					markTokenAsLoggedOut(suite.AuthHeader)
				}
				passed++
			} else {
				failMsg := fmt.Sprintf("❌ 测试失败 - %s", suite.CaseID)
				fmt.Println(withColor("  "+failMsg, colorBrightRed))

				// 显示失败详情
				fmt.Println(withColor("  失败详情:", colorRed+colorBold))
				if !statusPass {
					msg := fmt.Sprintf("  - HTTP状态码不匹配: 预期 %d, 实际 %d", suite.ExpectedStatus, actualStatus)
					fmt.Println(withColor(msg, colorRed))
				}
				if !codePass {
					msg := fmt.Sprintf("  - 业务码不匹配: 预期 %d, 实际 %d", suite.ExpectedCode, int(actualCode))
					fmt.Println(withColor(msg, colorRed))
				}
				if !msgPass {
					msg := fmt.Sprintf("  - 消息不匹配: 预期包含「%s」, 实际「%s」", suite.ExpectedMsg, actualMsg)
					fmt.Println(withColor(msg, colorRed))
				}
			}

			testResults = append(testResults, suite)
			total++
			printSeparator()
		})
	}

	// 7. 生成测试汇总报告
	fmt.Println(withColor("\n==============================================", colorBrightYellow+colorBold))
	fmt.Println(withColor("               测试汇总报告                  ", colorBrightYellow+colorBold))
	fmt.Println(withColor("==============================================", colorBrightYellow+colorBold))
	fmt.Printf("  %-20s %d\n", "总用例数:", total)
	fmt.Printf("  %-20s %s\n", "通过用例数:", withColor(fmt.Sprintf("%d", passed), colorBrightGreen))
	fmt.Printf("  %-20s %s\n", "失败用例数:", withColor(fmt.Sprintf("%d", total-passed), colorBrightRed))
	fmt.Println("\n" + withColor("用例详情:", colorBold))

	for _, suite := range testResults {
		status := withColor("✅ 成功", colorGreen)
		if !suite.Passed {
			status = withColor("❌ 失败", colorRed)
		}
		fmt.Printf("  %-10s %-45s %s\n", suite.CaseID, suite.Desc, status)
	}

	fmt.Println(withColor("\n==============================================", colorBrightYellow+colorBold))

	// 整体结果判定
	if total != passed {
		fatalMsg := fmt.Sprintf("测试未全部通过，失败用例数: %d", total-passed)
		t.Fatalf("%s", withColor(fatalMsg, colorBrightRed+colorBold))
	} else {
		fmt.Println(withColor("\n🎉 所有测试用例全部通过！", colorBrightGreen+colorBold))
	}
}
