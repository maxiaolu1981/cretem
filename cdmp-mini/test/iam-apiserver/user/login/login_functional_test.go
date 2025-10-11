package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

const testDir = "test/iam-apiserver/user/login"

// go test → 调用 TestMain(m *testing.M) → 由开发者控制测试执行
func TestMain(m *testing.M) {
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] export IAM_APISERVER_E2E=1 to run login e2e tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLoginFunctional(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "login")
	defer recorder.Flush(t)

	scenarios := []loginScenario{
		{
			name:        "success_basic_flow",
			description: "正常登录返回令牌并可访问受保护资源",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_ok_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected http=%d code=%d message=%s", resp.HTTPStatus(), resp.Code, resp.Message)
				}
				payload, err := decodeLoginPayload(resp)
				if err != nil {
					return framework.CaseResult{}, err
				}

				checks := map[string]bool{
					"access_token_issued":  payload.AccessToken != "",
					"refresh_token_issued": payload.RefreshToken != "",
					"operator_matches":     payload.Operator == spec.Name,
				}

				userResp, err := env.GetUser(payload.AccessToken, spec.Name)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("get user with token failed: %w", err)
				}
				if userResp.HTTPStatus() != http.StatusOK {
					return framework.CaseResult{}, fmt.Errorf("unexpected get user status=%d code=%d", userResp.HTTPStatus(), userResp.Code)
				}
				checks["token_authorized"] = true
				checks["session_persistent"] = payload.AccessToken != "" && payload.RefreshToken != ""

				return framework.CaseResult{
					Name:        "success_basic_flow",
					Description: "正确用户名密码应返回访问令牌并可访问用户信息",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
					Notes: []string{
						"验证 access/refresh token 发放",
						"使用 access token 调用 /v1/users/{name}",
					},
				}, nil
			},
		},
		{
			name:        "wrong_password",
			description: "错误密码应返回未授权",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_wrong_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, "WrongPass@123")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.HTTPStatus() != http.StatusUnauthorized || resp.Code != code.ErrPasswordIncorrect {
					return framework.CaseResult{}, fmt.Errorf("expect unauthorized wrong password, got http=%d code=%d", resp.HTTPStatus(), resp.Code)
				}
				return framework.CaseResult{
					Name:        "wrong_password",
					Description: "错误密码返回未授权",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks: map[string]bool{
						"invalid_credentials_rejected": true,
					},
				}, nil
			},
		},
		{
			name:        "nonexistent_user",
			description: "不存在的用户名应返回用户不存在",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				start := time.Now()
				username := env.RandomUsername("missing_")
				resp, err := loginRequest(env, username, "ValidPass#123")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrUserNotFound {
					return framework.CaseResult{}, fmt.Errorf("expected ErrUserNotFound, got code=%d message=%s", resp.Code, resp.Message)
				}
				return framework.CaseResult{
					Name:        "nonexistent_user",
					Description: "不存在的用户登录失败",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"user_not_found": true},
				}, nil
			},
		},
		{
			name:        "disabled_user",
			description: "禁用用户无法登录",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_disabled_", password)
				spec.Status = 0
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrUserDisabled {
					return framework.CaseResult{}, fmt.Errorf("expected ErrUserDisabled, got http=%d code=%d message=%s", resp.HTTPStatus(), resp.Code, resp.Message)
				}
				return framework.CaseResult{
					Name:        "disabled_user",
					Description: "禁用用户无法登录",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"disabled_blocked": true},
				}, nil
			},
		},
		{
			name:        "empty_username",
			description: "用户名为空应返回参数错误",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				start := time.Now()
				resp, err := loginRequest(env, "", "Passw0rd!23")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter, got code=%d", resp.Code)
				}
				return framework.CaseResult{
					Name:        "empty_username",
					Description: "用户名为空被拒绝",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"input_validation": true},
				}, nil
			},
		},
		{
			name:        "empty_password",
			description: "密码为空应返回参数错误",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_empty_pwd_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, "")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter, got code=%d", resp.Code)
				}
				return framework.CaseResult{
					Name:        "empty_password",
					Description: "空密码被拒绝",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"password_empty_rejected": true},
				}, nil
			},
		},
		{
			name:        "oversize_username",
			description: "超长用户名被拦截",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				overLong := strings.Repeat("a", 260)
				start := time.Now()
				resp, err := loginRequest(env, overLong, "ValidPass#123")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter for oversize username, got code=%d message=%s", resp.Code, resp.Message)
				}
				return framework.CaseResult{
					Name:        "oversize_username",
					Description: "超长用户名触发输入校验",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"length_validation": true},
				}, nil
			},
		},
		{
			name:        "sql_injection_payload",
			description: "SQL 注入 payload 被识别并拒绝",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				payload := "' OR '1'='1"
				start := time.Now()
				resp, err := loginRequest(env, payload, "ValidPass#123")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter for sql injection, got code=%d", resp.Code)
				}
				return framework.CaseResult{
					Name:        "sql_injection_payload",
					Description: "SQL 注入 payload 被拦截",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"sql_injection_blocked": true},
				}, nil
			},
		},
		{
			name:        "xss_payload",
			description: "XSS payload 被识别并拒绝",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				payload := "<script>alert(1)</script>"
				start := time.Now()
				resp, err := loginRequest(env, payload, "ValidPass#123")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter for xss, got code=%d", resp.Code)
				}
				return framework.CaseResult{
					Name:        "xss_payload",
					Description: "XSS payload 被拦截",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"xss_blocked": true},
				}, nil
			},
		},
		{
			name:        "special_character_password",
			description: "包含特殊字符的密码仍可成功登录",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "P@ss!#2025?"
				spec := env.NewUserSpec("login_special_pwd_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("special char login unexpected http=%d code=%d", resp.HTTPStatus(), resp.Code)
				}
				payload, err := decodeLoginPayload(resp)
				if err != nil {
					return framework.CaseResult{}, err
				}
				checks := map[string]bool{
					"tokens_issued": payload.AccessToken != "" && payload.RefreshToken != "",
				}
				return framework.CaseResult{
					Name:        "special_character_password",
					Description: "含特殊字符密码成功登录",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
				}, nil
			},
		},
		{
			name:        "consecutive_failures_lockout",
			description: "连续失败触发锁定提示",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_lock_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				var resp *framework.APIResponse
				for i := 0; i < 6; i++ {
					var err error
					resp, err = loginRequest(env, spec.Name, "BadPass!123")
					if err != nil {
						return framework.CaseResult{}, fmt.Errorf("attempt %d error: %w", i, err)
					}
				}
				if resp.Code != code.ErrPasswordIncorrect {
					return framework.CaseResult{}, fmt.Errorf("expected password incorrect code after lock, got %d", resp.Code)
				}
				if !strings.Contains(resp.Message, "登录失败次数太多") {
					return framework.CaseResult{}, fmt.Errorf("lockout message missing: %s", resp.Message)
				}
				return framework.CaseResult{
					Name:        "consecutive_failures_lockout",
					Description: "连续失败超过阈值提示锁定",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks: map[string]bool{
						"lockout_triggered": true,
					},
				}, nil
			},
		},
		{
			name:        "weak_password_complexity",
			description: "弱密码不满足复杂度要求被拒绝",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_complexity_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp, err := loginRequest(env, spec.Name, "weak")
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp.Code != code.ErrInvalidParameter {
					return framework.CaseResult{}, fmt.Errorf("expected invalid parameter for weak password, got %d", resp.Code)
				}
				if !strings.Contains(resp.Message, "密码不合法") {
					return framework.CaseResult{}, fmt.Errorf("unexpected message: %s", resp.Message)
				}
				return framework.CaseResult{
					Name:        "weak_password_complexity",
					Description: "弱密码复杂度被拒",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"password_policy_enforced": true},
				}, nil
			},
		},
		{
			name:        "multi_session_tokens",
			description: "多设备登录返回不同令牌且均可访问",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_multi_session_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				resp1, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp1.HTTPStatus() != http.StatusOK || resp1.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("first login unexpected http=%d code=%d message=%s", resp1.HTTPStatus(), resp1.Code, resp1.Message)
				}
				resp2, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if resp2.HTTPStatus() != http.StatusOK || resp2.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("second login unexpected http=%d code=%d message=%s", resp2.HTTPStatus(), resp2.Code, resp2.Message)
				}
				payload1, err := decodeLoginPayload(resp1)
				if err != nil {
					return framework.CaseResult{}, err
				}
				payload2, err := decodeLoginPayload(resp2)
				if err != nil {
					return framework.CaseResult{}, err
				}
				if payload1.AccessToken == payload2.AccessToken {
					return framework.CaseResult{}, errors.New("access tokens should differ across sessions")
				}
				checks := map[string]bool{
					"distinct_tokens": payload1.AccessToken != payload2.AccessToken,
				}
				for i, token := range []string{payload1.AccessToken, payload2.AccessToken} {
					userResp, err := env.GetUser(token, spec.Name)
					if err != nil {
						return framework.CaseResult{}, fmt.Errorf("get user with token %d failed: %w", i, err)
					}
					if userResp.HTTPStatus() != http.StatusOK {
						return framework.CaseResult{}, fmt.Errorf("token %d not accepted: http=%d code=%d", i, userResp.HTTPStatus(), userResp.Code)
					}
				}
				checks["multi_session_access"] = true

				return framework.CaseResult{
					Name:        "multi_session_tokens",
					Description: "多设备登录产生独立会话",
					Success:     true,
					HTTPStatus:  resp2.HTTPStatus(),
					Code:        resp2.Code,
					Message:     resp2.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
				}, nil
			},
		},
		{
			name:        "refresh_token_flow",
			description: "刷新令牌可获取新的访问令牌",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_refresh_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				loginResp, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				payload, err := decodeLoginPayload(loginResp)
				if err != nil {
					return framework.CaseResult{}, err
				}

				refreshResp, err := env.Refresh(payload.AccessToken, payload.RefreshToken)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("refresh call failed: %w", err)
				}
				if refreshResp.HTTPStatus() != http.StatusOK || refreshResp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected refresh result http=%d code=%d message=%s", refreshResp.HTTPStatus(), refreshResp.Code, refreshResp.Message)
				}
				return framework.CaseResult{
					Name:        "refresh_token_flow",
					Description: "刷新令牌成功获取新令牌",
					Success:     true,
					HTTPStatus:  refreshResp.HTTPStatus(),
					Code:        refreshResp.Code,
					Message:     refreshResp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      map[string]bool{"refresh_success": true},
				}, nil
			},
		},
		{
			name:        "logout_revokes_token",
			description: "注销后旧令牌无法访问",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_logout_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				loginResp, err := loginRequest(env, spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, err
				}
				payload, err := decodeLoginPayload(loginResp)
				if err != nil {
					return framework.CaseResult{}, err
				}

				logoutResp, err := env.Logout(payload.AccessToken, payload.RefreshToken)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("logout call failed: %w", err)
				}
				if logoutResp.HTTPStatus() != http.StatusOK || logoutResp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected logout result http=%d code=%d", logoutResp.HTTPStatus(), logoutResp.Code)
				}

				secResp, err := env.GetUser(payload.AccessToken, spec.Name)
				if err == nil && secResp.HTTPStatus() == http.StatusOK {
					return framework.CaseResult{}, errors.New("expect revoked access token to fail after logout")
				}
				checks := map[string]bool{"token_revoked": true}

				return framework.CaseResult{
					Name:        "logout_revokes_token",
					Description: "注销后 access token 失效",
					Success:     true,
					HTTPStatus:  logoutResp.HTTPStatus(),
					Code:        logoutResp.Code,
					Message:     logoutResp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
				}, nil
			},
		},
		{
			name:        "token_expiry_short_ttl",
			description: "调试 TTL 覆盖后令牌按期失效",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_expire_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				tokens, resp, err := env.LoginWithOptions(spec.Name, password, &framework.LoginOptions{
					Headers: map[string]string{
						"X-Debug-Token-Timeout": "1s",
					},
				})
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("login with short ttl: %w", err)
				}
				if resp == nil || resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected login response http=%d code=%d message=%s", resp.HTTPStatus(), resp.Code, resp.Message)
				}

				preResp, err := env.GetUser(tokens.AccessToken, spec.Name)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("verify token before expiry: %w", err)
				}
				if preResp.HTTPStatus() != http.StatusOK {
					return framework.CaseResult{}, fmt.Errorf("token should be valid before expiry, got http=%d code=%d", preResp.HTTPStatus(), preResp.Code)
				}

				time.Sleep(2 * time.Second)

				postResp, err := env.GetUser(tokens.AccessToken, spec.Name)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("verify token after expiry: %w", err)
				}
				if postResp.HTTPStatus() != http.StatusUnauthorized || postResp.Code != code.ErrExpired {
					return framework.CaseResult{}, fmt.Errorf("expected expired token unauthorized, got http=%d code=%d message=%s", postResp.HTTPStatus(), postResp.Code, postResp.Message)
				}

				checks := map[string]bool{
					"short_ttl_override": true,
					"pre_expiry_access":  true,
					"expired_rejected":   true,
				}

				return framework.CaseResult{
					Name:        "token_expiry_short_ttl",
					Description: "通过调试 TTL 获取短期令牌并验证过期行为",
					Success:     true,
					HTTPStatus:  resp.HTTPStatus(),
					Code:        resp.Code,
					Message:     resp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
					Notes: []string{
						"登录时使用 X-Debug-Token-Timeout=1s",
						"等待超过 TTL 后访问受保护资源应返回令牌过期",
					},
				}, nil
			},
		},
		{
			name:        "audit_login_events",
			description: "登录行为记录成功与失败审计日志",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_audit_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()

				failResp, err := loginRequest(env, spec.Name, "WrongPass@123")
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("login fail attempt: %w", err)
				}
				if failResp.Code != code.ErrPasswordIncorrect {
					return framework.CaseResult{}, fmt.Errorf("expected password incorrect code, got http=%d code=%d message=%s", failResp.HTTPStatus(), failResp.Code, failResp.Message)
				}

				_, successResp, err := env.Login(spec.Name, password)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("login success attempt: %w", err)
				}
				if successResp == nil || successResp.HTTPStatus() != http.StatusOK || successResp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected success login response http=%d code=%d message=%s", successResp.HTTPStatus(), successResp.Code, successResp.Message)
				}

				time.Sleep(200 * time.Millisecond)

				events, enabled, auditResp, err := env.AuditEvents(50)
				if err != nil {
					return framework.CaseResult{}, fmt.Errorf("fetch audit events: %w", err)
				}
				if auditResp == nil || auditResp.HTTPStatus() != http.StatusOK || auditResp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected audit response http=%d code=%d message=%s", auditResp.HTTPStatus(), auditResp.Code, auditResp.Message)
				}
				if !enabled {
					return framework.CaseResult{}, errors.New("audit manager reported disabled")
				}

				failRecorded := false
				successRecorded := false
				routeCaptured := false
				for _, event := range events {
					if event.Action != "auth.login" || event.ResourceID != spec.Name {
						continue
					}
					switch event.Outcome {
					case "fail":
						failRecorded = true
					case "success":
						successRecorded = true
					}
					if event.Metadata != nil {
						if route, ok := event.Metadata["route"].(string); ok && route == "/login" {
							routeCaptured = true
						}
					}
					if failRecorded && successRecorded && routeCaptured {
						break
					}
				}

				if !failRecorded || !successRecorded {
					return framework.CaseResult{}, fmt.Errorf("expected both success and failure audit events, success=%t fail=%t", successRecorded, failRecorded)
				}

				checks := map[string]bool{
					"login_fail_audited":    failRecorded,
					"login_success_audited": successRecorded,
					"route_recorded":        routeCaptured,
				}

				return framework.CaseResult{
					Name:        "audit_login_events",
					Description: "登录接口审计事件包含失败与成功记录",
					Success:     true,
					HTTPStatus:  auditResp.HTTPStatus(),
					Code:        auditResp.Code,
					Message:     auditResp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
					Notes: []string{
						"校验 /admin/audit/events 中的 auth.login 成功与失败事件",
					},
				}, nil
			},
		},
		{
			name:        "login_rate_limit_enforced",
			description: "超过登录限流阈值返回429",
			run: func(t *testing.T, env *framework.Env) (framework.CaseResult, error) {
				t.Helper()
				const password = "InitPassw0rd!"
				spec := env.NewUserSpec("login_ratelimit_", password)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				defer env.ForceDeleteUserIgnore(spec.Name)

				start := time.Now()
				if resp, err := env.SetLoginRateLimit(1); err != nil {
					return framework.CaseResult{}, fmt.Errorf("set login limit: %w", err)
				} else if resp == nil || resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					return framework.CaseResult{}, fmt.Errorf("unexpected set limit response http=%d code=%d message=%s", resp.HTTPStatus(), resp.Code, resp.Message)
				}
				defer env.ResetLoginRateLimit()

				time.Sleep(100 * time.Millisecond)

				limited := false
				limitedAttempt := 0
				var limitedResp *framework.APIResponse
				attemptNotes := make([]string, 0, 5)

				for i := 1; i <= 5; i++ {
					resp, err := loginRequest(env, spec.Name, "WrongPass@123")
					if err != nil {
						return framework.CaseResult{}, fmt.Errorf("attempt %d request error: %w", i, err)
					}
					attemptNotes = append(attemptNotes, fmt.Sprintf("#%d http=%d code=%d", i, resp.HTTPStatus(), resp.Code))
					if resp.HTTPStatus() == http.StatusTooManyRequests && resp.Code == 100209 {
						limited = true
						limitedAttempt = i
						limitedResp = resp
						break
					}
					// 小延迟避免瞬间刷爆 redis
					time.Sleep(50 * time.Millisecond)
				}

				if !limited {
					return framework.CaseResult{}, errors.New("expected to hit login rate limit within 5 attempts")
				}

				messageContains := false
				if limitedResp != nil {
					messageContains = strings.Contains(limitedResp.Message, "请求过于频繁")
				}

				checks := map[string]bool{
					"rate_limit_triggered":        limited,
					"limit_message_includes_hint": messageContains,
					"limit_attempt_within_5":      limitedAttempt > 0 && limitedAttempt <= 5,
				}

				notes := []string{fmt.Sprintf("limit hit on attempt %d", limitedAttempt)}
				notes = append(notes, attemptNotes...)

				return framework.CaseResult{
					Name:        "login_rate_limit_enforced",
					Description: "登录限流阈值设置为1后，第3次以内请求触发429",
					Success:     true,
					HTTPStatus:  http.StatusTooManyRequests,
					Code:        100209,
					Message:     limitedResp.Message,
					DurationMS:  time.Since(start).Milliseconds(),
					Checks:      checks,
					Notes:       notes,
				}, nil
			},
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			res, err := scenario.run(t, env)
			if err != nil {
				t.Fatalf("login scenario %s failed: %v", scenario.name, err)
			}
			recorder.AddCase(res)
		})
	}
}

type loginScenario struct {
	name        string
	description string
	run         func(t *testing.T, env *framework.Env) (framework.CaseResult, error)
}

type loginPayload struct {
	LoginUser     string `json:"login_user"`
	Operator      string `json:"operator"`
	OperationTime string `json:"operation_time"`
	OperationType string `json:"operation_type"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	Expire        string `json:"expire"`
	TokenType     string `json:"token_type"`
}

func loginRequest(env *framework.Env, username, password string) (*framework.APIResponse, error) {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	return env.AuthorizedRequest(http.MethodPost, "/login", "", payload)
}

func decodeLoginPayload(resp *framework.APIResponse) (*loginPayload, error) {
	if resp == nil {
		return nil, errors.New("response is nil")
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("login response missing data payload")
	}
	var payload loginPayload
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return nil, fmt.Errorf("decode login payload: %w", err)
	}
	return &payload, nil
}
