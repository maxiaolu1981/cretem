package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
)

// -------------------------- 1. 基础配置 --------------------------
const (
	API_PATH             = "/refresh"
	ALLOWED_HTTP_METHOD  = http.MethodPost
	CONTENT_TYPE_JSON    = "application/json"
	TOKEN_TYPE_BEARER    = "Bearer"
	CORS_ALLOWED_ORIGIN  = "http://localhost:3000"
	ACCESS_TOKEN_EXPIRE  = 1 * time.Hour
	REFRESH_TOKEN_EXPIRE = 7 * 24 * time.Hour
)

// -------------------------- 2. 业务码常量 --------------------------
// 通用基本错误（1000xx）
const (
	CODE_SUCCESS           = 100001 // 成功
	CODE_UNKNOWN           = 100002 // 内部服务器错误
	CODE_BIND_FAILED       = 100003 // 请求体绑定失败
	CODE_VALIDATION_FAILED = 100004 // 语义校验失败
	CODE_NOT_FOUND         = 100005 // 资源不存在
	CODE_METHOD_NOT_ALLOW  = 100006 // 方法不允许
	CODE_UNSUPPORTED_MEDIA = 100007 // 不支持的Content-Type
)

// 通用授权认证错误（1002xx）
const (
	CODE_ERR_ENCRYPT               = 100201 // 密码加密失败
	CODE_ERR_SIGNATURE_INVALID     = 100202 // 签名无效
	CODE_ERR_EXPIRED               = 100203 // 令牌已过期
	CODE_ERR_INVALID_AUTH_HEADER   = 100204 // 授权头格式无效
	CODE_ERR_MISSING_AUTH_HEADER   = 100205 // 缺少Authorization头
	CODE_ERR_PASSWORD_INCORRECT    = 100206 // 密码不正确
	CODE_ERR_PERMISSION_DENIED     = 100207 // 权限不足
	CODE_ERR_TOKEN_INVALID         = 100208 // 令牌无效（格式/签名错误）
	CODE_ERR_BASE64_DECODE_FAIL    = 100209 // Base64解码失败
	CODE_ERR_INVALID_BASIC_PAYLOAD = 100210 // Basic认证格式无效
)

// 通用加解码错误（1003xx）
const (
	CODE_ERR_INVALID_JSON = 100303 // 无效JSON格式
)

// HTTP状态码映射
const (
	HTTP_200 = http.StatusOK
	HTTP_201 = http.StatusCreated
	HTTP_400 = http.StatusBadRequest
	HTTP_401 = http.StatusUnauthorized
	HTTP_403 = http.StatusForbidden
	HTTP_404 = http.StatusNotFound
	HTTP_405 = http.StatusMethodNotAllowed
	HTTP_415 = http.StatusUnsupportedMediaType
	HTTP_500 = http.StatusInternalServerError
)

// -------------------------- 3. 子码定义 --------------------------
const (
	SUBCODE_NONE                = 0    // 无细分场景
	SUBCODE_TOKEN_IN_URL        = 1001 // URL传递Token
	SUBCODE_MISSING_REFRESH     = 1002 // 缺少refresh_token
	SUBCODE_INVALID_REFRESH     = 1003 // refresh_token无效
	SUBCODE_EXPIRED_REFRESH     = 1004 // refresh_token已过期
	SUBCODE_REVOKED_REFRESH     = 1005 // refresh_token已吊销
	SUBCODE_INVALID_JSON_FORMAT = 1006 // JSON格式错误
)

// -------------------------- 4. 核心数据结构 --------------------------
type JwtClaims struct {
	UserID    string `json:"user_id"`
	TokenType string `json:"token_type"` // "access"/"refresh"
	jwt.RegisteredClaims
}

var (
	revokedTokens = make(map[string]bool)
	mu            sync.RWMutex
)

// -------------------------- 5. 工具函数 --------------------------
func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "req-" + hex.EncodeToString(b)[:12]
}

func validateRefreshToken(tokenStr string) (error, int, int, int) {
	mu.RLock()
	defer mu.RUnlock()
	if revokedTokens[tokenStr] {
		return errors.New("refresh_token已吊销"), CODE_ERR_PERMISSION_DENIED, HTTP_403, SUBCODE_REVOKED_REFRESH
	}

	jwtSecret := []byte("test-jwt-secret-123")
	token, err := jwt.ParseWithClaims(tokenStr, &JwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("签名算法无效")
		}
		return jwtSecret, nil
	})

	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok && ve.Errors&jwt.ValidationErrorExpired != 0 {
			return errors.New("refresh_token已过期"), CODE_ERR_EXPIRED, HTTP_401, SUBCODE_EXPIRED_REFRESH
		}
		return errors.New("refresh_token格式或签名错误"), CODE_ERR_TOKEN_INVALID, HTTP_401, SUBCODE_INVALID_REFRESH
	}

	claims, ok := token.Claims.(*JwtClaims)
	if !ok || !token.Valid || claims.TokenType != "refresh" {
		return errors.New("refresh_token类型无效"), CODE_ERR_TOKEN_INVALID, HTTP_401, SUBCODE_INVALID_REFRESH
	}

	return nil, 0, 0, SUBCODE_NONE
}

// -------------------------- 6. 核心Handler --------------------------
func RefreshTokenHandler(c *gin.Context) {
	reqID := generateRequestID()
	c.Header("X-Request-ID", reqID)
	c.Header("Content-Type", CONTENT_TYPE_JSON)

	// 场景1：验证Content-Type
	if c.ContentType() != CONTENT_TYPE_JSON {
		c.JSON(HTTP_415, gin.H{
			"code":       CODE_UNSUPPORTED_MEDIA,
			"sub_code":   SUBCODE_NONE,
			"message":    "不支持的Content-Type，仅支持application/json",
			"request_id": reqID,
		})
		return
	}

	// 场景2：禁止URL传递Token
	if c.Request.URL.Query().Get("refresh_token") != "" {
		c.JSON(HTTP_400, gin.H{
			"code":       CODE_BIND_FAILED,
			"sub_code":   SUBCODE_TOKEN_IN_URL,
			"message":    "禁止URL传递refresh_token",
			"request_id": reqID,
		})
		return
	}

	// 场景3：授权头格式验证
	var refreshToken string
	authHeader := c.GetHeader("Authorization")
	hasAuthHeader := authHeader != "" // 声明hasAuthHeader变量

	if hasAuthHeader {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != TOKEN_TYPE_BEARER || strings.TrimSpace(parts[1]) == "" {
			c.JSON(HTTP_400, gin.H{
				"code":       CODE_ERR_INVALID_AUTH_HEADER,
				"sub_code":   SUBCODE_NONE,
				"message":    "授权头格式无效，需为：Bearer <refresh_token>（token不能为空）",
				"request_id": reqID,
			})
			return
		}
		refreshToken = parts[1]
	}

	// 场景4：无授权头时从请求体获取
	if !hasAuthHeader {
		var reqBody struct {
			RefreshToken string `json:"refresh_token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			// 准确判断JSON格式错误
			var jsonErr *json.SyntaxError
			if errors.As(err, &jsonErr) {
				c.JSON(HTTP_400, gin.H{
					"code":       CODE_ERR_INVALID_JSON,
					"sub_code":   SUBCODE_INVALID_JSON_FORMAT,
					"message":    fmt.Sprintf("无效JSON格式：%v", jsonErr.Error()),
					"request_id": reqID,
				})
			} else if strings.Contains(err.Error(), "invalid character") ||
				strings.Contains(err.Error(), "unexpected EOF") ||
				strings.Contains(err.Error(), "invalid JSON") {
				c.JSON(HTTP_400, gin.H{
					"code":       CODE_ERR_INVALID_JSON,
					"sub_code":   SUBCODE_INVALID_JSON_FORMAT,
					"message":    "无效JSON格式（如括号缺失、逗号错误）",
					"request_id": reqID,
				})
			} else {
				c.JSON(HTTP_400, gin.H{
					"code":       CODE_BIND_FAILED,
					"sub_code":   SUBCODE_MISSING_REFRESH,
					"message":    "缺少refresh_token字段",
					"request_id": reqID,
				})
			}
			return
		}
		refreshToken = reqBody.RefreshToken
	}

	// 场景5：Token有效性验证
	err, errCode, httpStatus, subCode := validateRefreshToken(refreshToken)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"code":       errCode,
			"sub_code":   subCode,
			"message":    err.Error(),
			"request_id": reqID,
		})
		return
	}

	// 场景6：生成新Token
	newAccessToken, err := generateJwtToken("access", false)
	if err != nil {
		c.JSON(HTTP_500, gin.H{
			"code":       CODE_UNKNOWN,
			"sub_code":   SUBCODE_NONE,
			"message":    "生成access_token失败",
			"request_id": reqID,
		})
		return
	}
	newRefreshToken, err := generateJwtToken("refresh", false)
	if err != nil {
		c.JSON(HTTP_500, gin.H{
			"code":       CODE_UNKNOWN,
			"sub_code":   SUBCODE_NONE,
			"message":    "生成refresh_token失败",
			"request_id": reqID,
		})
		return
	}
	revokeToken(refreshToken)

	// 场景7：成功响应
	c.JSON(HTTP_201, gin.H{
		"code":       CODE_SUCCESS,
		"sub_code":   SUBCODE_NONE,
		"message":    "Token刷新成功",
		"request_id": reqID,
		"data": gin.H{
			"access_token":  newAccessToken,
			"refresh_token": newRefreshToken,
			"token_type":    TOKEN_TYPE_BEARER,
			"expires_in":    int(ACCESS_TOKEN_EXPIRE.Seconds()),
		},
	})
}

// -------------------------- 7. 路由初始化 --------------------------
func initRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	gin.DisableConsoleColor() // 禁用Gin彩色日志，避免干扰测试输出
	r := gin.Default()

	// 404处理器
	r.NoRoute(func(c *gin.Context) {
		reqID := generateRequestID()
		c.Header("X-Request-ID", reqID)
		c.Header("Content-Type", CONTENT_TYPE_JSON)
		c.JSON(HTTP_404, gin.H{
			"code":       CODE_NOT_FOUND,
			"sub_code":   SUBCODE_NONE,
			"message":    "资源不存在",
			"request_id": reqID,
		})
	})

	// 405处理器
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		reqID := generateRequestID()
		c.Header("X-Request-ID", reqID)
		c.Header("Content-Type", CONTENT_TYPE_JSON)
		c.JSON(HTTP_405, gin.H{
			"code":       CODE_METHOD_NOT_ALLOW,
			"sub_code":   SUBCODE_NONE,
			"message":    "仅支持POST方法",
			"request_id": reqID,
		})
	})

	// CORS配置
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{CORS_ALLOWED_ORIGIN},
		AllowMethods:     []string{ALLOWED_HTTP_METHOD},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
	}))

	r.POST(API_PATH, RefreshTokenHandler)
	return r
}

// -------------------------- 8. 测试辅助函数 --------------------------
func copyResponseBody(resp *http.Response) ([]byte, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes, nil
}

func printServerResp(resp *http.Response) (int, int, int, string) {
	bodyBytes, err := copyResponseBody(resp)
	if err != nil {
		color.Cyan("[服务器返回] 读取失败：%v\n", err)
		return 0, 0, 0, ""
	}

	var respJson map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respJson); err != nil {
		color.Cyan("[服务器返回] 非JSON格式：%s\n", string(bodyBytes))
		return resp.StatusCode, 0, 0, string(bodyBytes)
	}

	httpStatus := resp.StatusCode
	code := int(respJson["code"].(float64))
	subCode := int(respJson["sub_code"].(float64))
	message := respJson["message"].(string)

	color.Cyan(
		"[服务器返回] HTTP状态：%d | 业务码：%d | 子码：%d | 消息：%s\n",
		httpStatus, code, subCode, message,
	)
	return httpStatus, code, subCode, message
}

func validateResp(resp *http.Response, expectHTTP, expectCode, expectSubCode int) error {
	httpStatus, code, subCode, _ := printServerResp(resp)

	if httpStatus != expectHTTP {
		return fmt.Errorf("HTTP状态不匹配：预期%d，实际%d", expectHTTP, httpStatus)
	}
	if code != expectCode {
		return fmt.Errorf("业务码不匹配：预期%d，实际%d", expectCode, code)
	}
	if subCode != expectSubCode {
		return fmt.Errorf("子码不匹配：预期%d，实际%d", expectSubCode, subCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), CONTENT_TYPE_JSON) {
		return fmt.Errorf("Content-Type错误：预期%s，实际%s", CONTENT_TYPE_JSON, resp.Header.Get("Content-Type"))
	}

	return nil
}

// -------------------------- 9. 测试结果工具 --------------------------
var (
	total   int
	passed  int
	failed  int
	results []string
)

func recordTestResult(caseID, caseName string, isPass bool, failReason string) {
	total++
	if isPass {
		passed++
	} else {
		failed++
	}
	results = append(results, fmt.Sprintf("%s：%s → %s", caseID, caseName, map[bool]string{true: "通过 ✅", false: "失败 ❌"}[isPass]))
}

func printTestResult(caseID, caseName string, isPass bool, failReason string) {
	fmt.Println(strings.Repeat("-", 120))
	if isPass {
		color.Green("[%s] %s → 通过 ✅\n", caseID, caseName)
	} else {
		color.Red("[%s] %s → 失败 ❌\n失败原因：%s\n", caseID, caseName, failReason)
	}
	color.Unset()
}

// -------------------------- 10. 测试用例 --------------------------
func TestRule1_URLUniqueness(t *testing.T) {
	t.Parallel()
	caseID := "RULE-001"
	caseName := "错误路径返回100005（404）"
	router := initRouter()
	clearRevokedTokens()

	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, "/refresh2", bytes.NewBufferString(`{"refresh_token":"test"}`))
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_404, CODE_NOT_FOUND, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule2_OnlyPostAllowed(t *testing.T) {
	t.Parallel()
	caseID := "RULE-002"
	caseName := "非POST返回100006（405）"
	router := initRouter()
	clearRevokedTokens()

	reqGet := httptest.NewRequest(http.MethodGet, API_PATH, nil)
	reqGet.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if err := validateResp(wGet.Result(), HTTP_405, CODE_METHOD_NOT_ALLOW, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule3_OnlyJsonAllowed(t *testing.T) {
	t.Parallel()
	caseID := "RULE-003"
	caseName := "非JSON返回100007（415）"
	router := initRouter()
	clearRevokedTokens()

	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, strings.NewReader("refresh_token=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_415, CODE_UNSUPPORTED_MEDIA, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule4_TokenNoUrl(t *testing.T) {
	t.Parallel()
	caseID := "RULE-004"
	caseName := "URL传Token返回100003（400）"
	router := initRouter()
	clearRevokedTokens()

	validToken, _ := generateJwtToken("refresh", false)
	reqUrl := API_PATH + "?refresh_token=" + validToken
	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, reqUrl, nil)
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_400, CODE_BIND_FAILED, SUBCODE_TOKEN_IN_URL); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule5_ValidAuthHeader(t *testing.T) {
	t.Parallel()
	caseID := "RULE-005"
	caseName := "无效授权头返回100204（400）"
	router := initRouter()
	clearRevokedTokens()
	validToken, _ := generateJwtToken("refresh", false)

	// 测试1：缺少Bearer前缀
	req1 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req1.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req1.Header.Set("Authorization", validToken)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if err := validateResp(w1.Result(), HTTP_400, CODE_ERR_INVALID_AUTH_HEADER, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, "测试1失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试1失败："+err.Error())
		t.Fatal(err)
	}

	// 测试2：前缀错误（Basic）
	req2 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req2.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req2.Header.Set("Authorization", "Basic "+validToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if err := validateResp(w2.Result(), HTTP_400, CODE_ERR_INVALID_AUTH_HEADER, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, "测试2失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试2失败："+err.Error())
		t.Fatal(err)
	}

	// 测试3：无空格分割
	req3 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req3.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req3.Header.Set("Authorization", "Bearer"+validToken)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if err := validateResp(w3.Result(), HTTP_400, CODE_ERR_INVALID_AUTH_HEADER, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, "测试3失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试3失败："+err.Error())
		t.Fatal(err)
	}

	// 测试4：Bearer+空Token
	req4 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req4.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req4.Header.Set("Authorization", TOKEN_TYPE_BEARER+" ")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if err := validateResp(w4.Result(), HTTP_400, CODE_ERR_INVALID_AUTH_HEADER, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, "测试4失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试4失败："+err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule6_ExpiredToken(t *testing.T) {
	t.Parallel()
	caseID := "RULE-006"
	caseName := "过期Token返回100203（401）"
	router := initRouter()
	clearRevokedTokens()

	expiredToken, _ := generateJwtToken("refresh", true)
	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req.Header.Set("Authorization", TOKEN_TYPE_BEARER+" "+expiredToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_401, CODE_ERR_EXPIRED, SUBCODE_EXPIRED_REFRESH); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule7_RevokedToken(t *testing.T) {
	t.Parallel()
	caseID := "RULE-007"
	caseName := "吊销Token返回100207（403）"
	router := initRouter()
	clearRevokedTokens()

	revokedToken, _ := generateJwtToken("refresh", false)
	revokeToken(revokedToken)
	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req.Header.Set("Authorization", TOKEN_TYPE_BEARER+" "+revokedToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_403, CODE_ERR_PERMISSION_DENIED, SUBCODE_REVOKED_REFRESH); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule8_InvalidJson(t *testing.T) {
	t.Parallel()
	caseID := "RULE-008"
	caseName := "无效JSON返回100303（400）"
	router := initRouter()
	clearRevokedTokens()

	// 测试1：缺少闭合}
	invalidJson1 := `{"refresh_token":"test"`
	req1 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, bytes.NewBufferString(invalidJson1))
	req1.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if err := validateResp(w1.Result(), HTTP_400, CODE_ERR_INVALID_JSON, SUBCODE_INVALID_JSON_FORMAT); err != nil {
		printTestResult(caseID, caseName, false, "测试1失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试1失败："+err.Error())
		t.Fatal(err)
	}

	// 测试2：多余逗号
	invalidJson2 := `{"refresh_token": "test",}`
	req2 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, bytes.NewBufferString(invalidJson2))
	req2.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if err := validateResp(w2.Result(), HTTP_400, CODE_ERR_INVALID_JSON, SUBCODE_INVALID_JSON_FORMAT); err != nil {
		printTestResult(caseID, caseName, false, "测试2失败："+err.Error())
		recordTestResult(caseID, caseName, false, "测试2失败："+err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

func TestRule10_SuccessRefresh(t *testing.T) {
	t.Parallel()
	caseID := "RULE-010"
	caseName := "成功刷新返回100001（201）"
	router := initRouter()
	clearRevokedTokens()

	validToken, _ := generateJwtToken("refresh", false)
	req := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req.Header.Set("Authorization", TOKEN_TYPE_BEARER+" "+validToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := validateResp(w.Result(), HTTP_201, CODE_SUCCESS, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, err.Error())
		recordTestResult(caseID, caseName, false, err.Error())
		t.Fatal(err)
	}

	bodyBytes, _ := copyResponseBody(w.Result())
	var respJson map[string]interface{}
	json.Unmarshal(bodyBytes, &respJson)
	dataVal, hasData := respJson["data"]
	if !hasData || dataVal == nil {
		failReason := "缺少data字段"
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	data, isMap := dataVal.(map[string]interface{})
	if !isMap {
		failReason := "data不是JSON对象"
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	if data["access_token"] == "" || data["refresh_token"] == "" {
		failReason := "缺少新Token"
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

// -------------------------- 11. 测试入口 --------------------------
func TestMain(m *testing.M) {
	color.NoColor = false
	color.Output = os.Stdout
	viper.Set("jwt.secret", "test-jwt-secret-123")
	clearRevokedTokens()

	exitCode := m.Run()

	fmt.Println("\n" + strings.Repeat("=", 120))
	color.Cyan("10项RESTful规则测试汇总（适配 /refresh + 业务码规范）")
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("总用例：%d | 通过：%d | 失败：%d\n", total, passed, failed)
	fmt.Println(strings.Repeat("-", 120))
	for _, res := range results {
		if strings.Contains(res, "失败") {
			color.Red(res)
		} else {
			color.Green(res)
		}
	}
	color.Unset()

	os.Exit(exitCode)
}

func generateJwtToken(tokenType string, isExpired bool) (string, error) {
	now := time.Now()
	var expireTime time.Time
	if tokenType == "access" {
		expireTime = now.Add(ACCESS_TOKEN_EXPIRE)
	} else if tokenType == "refresh" { // 严格校验tokenType，避免生成错误类型
		expireTime = now.Add(REFRESH_TOKEN_EXPIRE)
	} else {
		return "", errors.New("无效的token类型，仅支持access/refresh")
	}
	if isExpired {
		expireTime = now.Add(-1 * time.Hour)
	}

	claims := JwtClaims{
		UserID:    "test-user-123",
		TokenType: tokenType, // 确保类型正确写入claims
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			Issuer:    "iam-apiserver",
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	jwtSecret := []byte("test-jwt-secret-123")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// -------------------------- 2. 修复RULE-009测试用例（解决状态污染） --------------------------
func TestRule9_Idempotency(t *testing.T) {
	// 关键：禁用并行执行，避免与其他用例（如RULE-007）共享revokedTokens状态
	// t.Parallel() // 注释掉并行执行，确保状态隔离
	caseID := "RULE-009"
	caseName := "旧Token失效返回100207（403）"
	router := initRouter()
	clearRevokedTokens() // 强制清理吊销列表，确保初始状态干净

	// 生成全新的有效refresh_token（验证生成逻辑）
	oldToken, err := generateJwtToken("refresh", false)
	if err != nil {
		failReason := "生成有效refresh_token失败：" + err.Error()
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(err)
	}

	// 验证生成的Token初始状态（未被吊销）
	mu.RLock()
	isRevoked := revokedTokens[oldToken]
	mu.RUnlock()
	if isRevoked {
		failReason := "新生成的Token被意外标记为已吊销"
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	// 第一次刷新（应成功）
	req1 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req1.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req1.Header.Set("Authorization", TOKEN_TYPE_BEARER+" "+oldToken)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	// 验证第一次刷新的响应（核心修复点：确保返回201）
	if err := validateResp(w1.Result(), HTTP_201, CODE_SUCCESS, SUBCODE_NONE); err != nil {
		printTestResult(caseID, caseName, false, "第一次刷新失败："+err.Error())
		recordTestResult(caseID, caseName, false, "第一次刷新失败："+err.Error())
		t.Fatal(err)
	}

	// 验证旧Token已被吊销（第一次刷新后）
	mu.RLock()
	isRevokedAfterFirst := revokedTokens[oldToken]
	mu.RUnlock()
	if !isRevokedAfterFirst {
		failReason := "第一次刷新后，旧Token未被正确吊销"
		printTestResult(caseID, caseName, false, failReason)
		recordTestResult(caseID, caseName, false, failReason)
		t.Fatal(failReason)
	}

	// 第二次使用旧Token刷新（应失败）
	req2 := httptest.NewRequest(ALLOWED_HTTP_METHOD, API_PATH, nil)
	req2.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req2.Header.Set("Authorization", TOKEN_TYPE_BEARER+" "+oldToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if err := validateResp(w2.Result(), HTTP_403, CODE_ERR_PERMISSION_DENIED, SUBCODE_REVOKED_REFRESH); err != nil {
		printTestResult(caseID, caseName, false, "第二次刷新（旧Token）失败："+err.Error())
		recordTestResult(caseID, caseName, false, "第二次刷新（旧Token）失败："+err.Error())
		t.Fatal(err)
	}

	printTestResult(caseID, caseName, true, "")
	recordTestResult(caseID, caseName, true, "")
}

// -------------------------- 3. 修复全局状态管理（避免并发污染） --------------------------
// 在所有修改revokedTokens的地方添加更严格的锁控制
func revokeToken(tokenStr string) {
	mu.Lock()
	defer mu.Unlock()
	// 仅在token非空时执行吊销（避免空字符串污染）
	if tokenStr != "" {
		revokedTokens[tokenStr] = true
	}
}

func clearRevokedTokens() {
	mu.Lock()
	defer mu.Unlock()
	// 彻底重建map，避免残留引用
	revokedTokens = make(map[string]bool)
}
