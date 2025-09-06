package logout

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// -------------------------- 1. 基础定义（不变）--------------------------
const (
	ErrMissingHeader     = 100205 // 无令牌
	ErrInvalidAuthHeader = 100204 // 授权头格式错误
	ErrTokenInvalid      = 100208 // 令牌格式错误
	ErrExpired           = 100203 // 令牌过期
	ErrSignatureInvalid  = 100202 // 签名无效
	ErrUnauthorized      = 110003 // 未授权
	ErrInternal          = 50001  // 服务器内部错误
	SuccessCode          = 0      // 成功业务码
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

type CustomClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

var (
	tokenBlacklist = make(map[string]bool)
	mu             sync.Mutex
)

func destroyToken(tokenString string) error {
	mu.Lock()
	defer mu.Unlock()
	tokenBlacklist[tokenString] = true
	return nil
}
func isTokenBlacklisted(tokenString string) bool {
	mu.Lock()
	defer mu.Unlock()
	return tokenBlacklist[tokenString]
}
func trimBearerPrefix(token string) string {
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return strings.TrimPrefix(token, "Bearer ")
	}
	return token
}

// -------------------------- 2. 核心逻辑（不变）--------------------------
func ValidateToken(tokenString string) (*CustomClaims, error) {
	rawToken := trimBearerPrefix(tokenString)
	if isTokenBlacklisted(rawToken) {
		return nil, withCode(ErrUnauthorized, "令牌已登出，请重新登录")
	}

	var jwtSecret = []byte(viper.GetString("jwt.key"))
	if tokenString == "" {
		return nil, withCode(ErrMissingHeader, "请先登录")
	}
	tokenString = rawToken
	if tokenString == "" {
		return nil, withCode(ErrInvalidAuthHeader, "invalid authorization header format")
	}

	var claims CustomClaims
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, withCode(ErrTokenInvalid, "unsupported signing method")
			}
			return jwtSecret, nil
		},
	)

	if err != nil {
		_ = claims.UserID
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, withCode(ErrExpired, "令牌已过期")
		case errors.Is(err, jwt.ErrSignatureInvalid):
			return nil, withCode(ErrSignatureInvalid, "signature is invalid")
		default:
			if ve, ok := err.(*jwt.ValidationError); ok {
				if ve.Errors&jwt.ValidationErrorExpired != 0 {
					return nil, withCode(ErrExpired, "令牌已过期")
				}
				if ve.Errors&jwt.ValidationErrorSignatureInvalid != 0 {
					return nil, withCode(ErrSignatureInvalid, "signature is invalid")
				}
				if ve.Errors&jwt.ValidationErrorMalformed != 0 {
					return nil, withCode(ErrTokenInvalid, "令牌格式错误")
				}
			}
			if strings.Contains(err.Error(), "invalid number of segments") {
				return nil, withCode(ErrTokenInvalid, "令牌格式错误")
			}
			return nil, withCode(ErrUnauthorized, "authentication failed")
		}
	}

	if customClaims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return customClaims, nil
	}
	_ = claims.UserID
	return nil, withCode(ErrTokenInvalid, "token is invalid")
}

func LogoutHandler(c *gin.Context) {
	token := c.GetHeader("Authorization")
	claims, err := ValidateToken(token)
	if err != nil {
		code := getErrCode(err)
		statusCode := http.StatusBadRequest
		if code == ErrMissingHeader || code == ErrExpired || code == ErrUnauthorized {
			statusCode = http.StatusUnauthorized
		}
		c.JSON(statusCode, gin.H{
			"code":    code,
			"message": getErrMsg(err),
			"data":    nil,
		})
		return
	}

	if err := destroyToken(trimBearerPrefix(token)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    ErrInternal,
			"message": "登出失败，请重试",
			"data":    nil,
		})
		return
	}

	_ = claims.UserID
	c.JSON(http.StatusOK, gin.H{
		"code":    SuccessCode,
		"message": "登出成功",
		"data":    nil,
	})
}

// -------------------------- 3. 测试工具函数（不变）--------------------------
func generateTestToken(isExpired, isInvalidSign bool) (string, error) {
	jwtSecret := []byte(viper.GetString("jwt.key"))
	invalidSecret := []byte("wrong-secret-654321")

	claims := &CustomClaims{
		UserID: "test-user-123",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	if isExpired {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Second))
	}

	secret := jwtSecret
	if isInvalidSign {
		secret = invalidSecret
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func maskToken(token string) string {
	if len(token) <= 20 {
		return token
	}
	return token[:20] + "..."
}

// -------------------------- 4. 测试用例定义（新增 ExpectedMsg 字段）--------------------------
// 修复：新增 ExpectedMsg 字段，与 login 脚本一致，同时解决 actualMsg 未使用问题
type LogoutTestSuite struct {
	CaseID         string // 用例编号（LOGOUT-001）
	Desc           string // 用例描述
	AuthHeader     string // Authorization请求头
	ExpectedStatus int    // 预期HTTP状态码
	ExpectedCode   int    // 预期业务码
	ExpectedMsg    string // 预期消息（包含匹配）
	Passed         bool   // 执行结果
}

// -------------------------- 5. 核心测试函数（使用 actualMsg 进行消息验证）--------------------------
func TestLogoutAPI_All(t *testing.T) {
	viper.Set("jwt.key", "test-jwt-secret-123")
	gin.SetMode(gin.ReleaseMode)
	mu.Lock()
	tokenBlacklist = make(map[string]bool)
	mu.Unlock()

	// 预生成令牌
	normalToken, _ := generateTestToken(false, false)
	expiredToken, _ := generateTestToken(true, false)
	invalidSignToken, _ := generateTestToken(false, true)
	invalidFormatToken := "Bearer invalid-token-no-dot"

	// 定义测试用例（新增 ExpectedMsg，与 login 脚本对齐）
	testSuites := []*LogoutTestSuite{
		{
			CaseID:         "LOGOUT-001",
			Desc:           "无Authorization请求头",
			AuthHeader:     "",
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrMissingHeader,
			ExpectedMsg:    "请先登录", // 预期消息
		},
		{
			CaseID:         "LOGOUT-002",
			Desc:           "Authorization格式无效（无有效令牌内容）",
			AuthHeader:     "Bearer ",
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrInvalidAuthHeader,
			ExpectedMsg:    "invalid authorization header format", // 预期消息
		},
		{
			CaseID:         "LOGOUT-003",
			Desc:           "令牌格式错误（无.分隔）",
			AuthHeader:     invalidFormatToken,
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrTokenInvalid,
			ExpectedMsg:    "令牌格式错误", // 预期消息
		},
		{
			CaseID:         "LOGOUT-004",
			Desc:           "令牌已过期",
			AuthHeader:     "Bearer " + expiredToken,
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrExpired,
			ExpectedMsg:    "令牌已过期", // 预期消息（与用例4需求一致）
		},
		{
			CaseID:         "LOGOUT-005",
			Desc:           "令牌签名无效",
			AuthHeader:     "Bearer " + invalidSignToken,
			ExpectedStatus: http.StatusBadRequest,
			ExpectedCode:   ErrSignatureInvalid,
			ExpectedMsg:    "signature is invalid", // 预期消息
		},
		{
			CaseID:         "LOGOUT-006",
			Desc:           "正常令牌登出成功",
			AuthHeader:     "Bearer " + normalToken,
			ExpectedStatus: http.StatusOK,
			ExpectedCode:   SuccessCode,
			ExpectedMsg:    "登出成功", // 预期消息
		},
		{
			CaseID:         "LOGOUT-007",
			Desc:           "已登出令牌再次登出",
			AuthHeader:     "Bearer " + normalToken,
			ExpectedStatus: http.StatusUnauthorized,
			ExpectedCode:   ErrUnauthorized,
			ExpectedMsg:    "令牌已登出", // 预期消息
		},
	}

	// 启动GIN服务
	r := gin.Default()
	r.DELETE("/logout", LogoutHandler)
	var total, passed int
	testResults := make([]*LogoutTestSuite, 0, len(testSuites))

	// 执行每个测试用例（核心修复：使用 actualMsg 进行消息验证）
	for _, suite := range testSuites {
		t.Run(suite.CaseID, func(t *testing.T) {
			// 1. 打印用例开始信息（新增预期消息打印）
			fmt.Printf("[%s] 开始执行：%s\n", suite.CaseID, suite.Desc)
			requestURL := "http://localhost:8080/logout"
			fmt.Printf("请求URL：%s\n", requestURL)
			fmt.Printf("预期：HTTP %d | 业务码 %d | 消息包含「%s」\n",
				suite.ExpectedStatus, suite.ExpectedCode, suite.ExpectedMsg)
			if suite.AuthHeader == "" {
				fmt.Println("Token：无")
			} else {
				fmt.Printf("Token：%s\n", maskToken(suite.AuthHeader))
			}

			// 2. 发送请求
			req := httptest.NewRequest("DELETE", requestURL, nil)
			if suite.AuthHeader != "" {
				req.Header.Set("Authorization", suite.AuthHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// 3. 解析响应（actualMsg 在此处声明并使用）
			var respBody map[string]interface{}
			_ = json.Unmarshal(w.Body.Bytes(), &respBody)
			actualStatus := w.Code
			actualCode := int(respBody["code"].(float64))
			actualMsg := respBody["message"].(string) // 声明 actualMsg

			// 4. 打印实际响应（新增实际消息打印）
			fmt.Printf("[%s] 实际响应：HTTP %d | 业务码 %d | 消息「%s」\n",
				suite.CaseID, actualStatus, actualCode, actualMsg)
			rawResp, _ := json.MarshalIndent(respBody, "", "  ")
			fmt.Printf("[%s] 原始响应体：%s\n", suite.CaseID, string(rawResp))
			fmt.Println("=================================================================================")

			// 5. 断言（核心：使用 actualMsg 验证消息，解决未使用问题）
			statusPass := assert.Equal(t, suite.ExpectedStatus, actualStatus, "[%s] HTTP状态码不符", suite.CaseID)
			codePass := assert.Equal(t, suite.ExpectedCode, actualCode, "[%s] 业务码不符", suite.CaseID)
			// 新增：验证消息是否包含预期内容（与 login 脚本一致）
			msgPass := assert.Contains(t, actualMsg, suite.ExpectedMsg, "[%s] 消息不符：预期包含「%s」，实际「%s」",
				suite.CaseID, suite.ExpectedMsg, actualMsg)

			// 6. 判断用例是否通过
			suite.Passed = statusPass && codePass && msgPass
			if suite.Passed {
				fmt.Printf("[%s] %s → 执行通过 ✅\n", suite.CaseID, suite.Desc)
				passed++
			} else {
				fmt.Printf("[%s] %s → 执行失败 ❌\n", suite.CaseID, suite.Desc)
				fmt.Printf("  预期：HTTP %d | 业务码 %d | 消息包含「%s」\n",
					suite.ExpectedStatus, suite.ExpectedCode, suite.ExpectedMsg)
				fmt.Printf("  实际：HTTP %d | 业务码 %d | 消息「%s」\n",
					actualStatus, actualCode, actualMsg) // 使用 actualMsg
			}
			testResults = append(testResults, suite)
			total++
			fmt.Println("---")
		})
	}

	// 7. 汇总报告（不变）
	fmt.Println("\n=================================================================================")
	fmt.Println("登出接口测试汇总报告")
	fmt.Println("=================================================================================")
	fmt.Printf("总用例数：%d\n", total)
	fmt.Printf("通过用例：%d\n", passed)
	fmt.Printf("失败用例：%d\n", total-passed)
	fmt.Println("---------------------------------------------------------------------------------")
	fmt.Println("用例执行详情：")
	for _, suite := range testResults {
		status := "✅ 执行通过"
		if !suite.Passed {
			status = "❌ 执行失败"
		}
		fmt.Printf("%s：%s → %s\n", suite.CaseID, suite.Desc, status)
	}
	fmt.Println("=================================================================================")

	if total == passed {
		fmt.Println("\n🎉 所有测试用例全部通过！")
	} else {
		t.Errorf("测试未全部通过，失败用例数：%d", total-passed)
	}
}
