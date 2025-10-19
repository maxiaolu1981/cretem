package get

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

const testDir = "test/iam-apiserver/user/get"

func TestMain(m *testing.M) {
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] export IAM_APISERVER_E2E=1 to run user get tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type getCase struct {
	name        string
	description string
	target      string
	tokenFunc   func(t *testing.T, env *framework.Env) string
	expectHTTP  int
	expectCode  int
	validate    func(t *testing.T, resp *framework.APIResponse)
}

func TestGetFunctional(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "get")
	defer recorder.Flush(t)

	const password = "InitPassw0rd!"

	spec := env.NewUserSpec("get_", password)
	env.CreateUserAndWait(t, spec, 5*time.Second)
	defer env.ForceDeleteUserIgnore(spec.Name)

	userTokens := env.LoginOrFail(t, spec.Name, password)
	missingUser := env.RandomUsername("missing_")

	cases := []getCase{
		{
			name:        "admin_get_existing",
			description: "管理员查询已存在用户应成功",
			target:      spec.Name,
			tokenFunc: func(t *testing.T, env *framework.Env) string {
				return env.AdminToken
			},
			expectHTTP: http.StatusOK,
			expectCode: code.ErrSuccess,
			validate: func(t *testing.T, resp *framework.APIResponse) {
				t.Helper()
				if len(resp.Data) == 0 {
					t.Fatalf("login before change failed: %v", fmt.Errorf("missing response payload"))
				}
				var payload map[string]any
				if err := json.Unmarshal(resp.Data, &payload); err != nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("decode response payload: %w", err))
				}
				value, ok := payload["get"].(string)
				if !ok || value != spec.Name {
					t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected payload get=%v", payload["get"]))
				}
			},
		},
		{
			name:        "user_get_self",
			description: "普通用户查询自身信息应成功",
			target:      spec.Name,
			tokenFunc: func(t *testing.T, env *framework.Env) string {
				return userTokens.AccessToken
			},
			expectHTTP: http.StatusOK,
			expectCode: code.ErrSuccess,
		},
		{
			name:        "user_not_found",
			description: "查询不存在用户应返回用户名密码无效错误",
			target:      missingUser,
			tokenFunc: func(t *testing.T, env *framework.Env) string {
				return env.AdminToken
			},
			expectHTTP: http.StatusUnauthorized,
			expectCode: code.ErrPasswordIncorrect,
		},
		{
			name:        "missing_token",
			description: "缺少授权头应返回缺少头错误",
			target:      spec.Name,
			tokenFunc: func(t *testing.T, env *framework.Env) string {
				return ""
			},
			expectHTTP: http.StatusUnauthorized,
			expectCode: code.ErrMissingHeader,
		},
		{
			name:        "invalid_token",
			description: "非法token应返回令牌无效",
			target:      spec.Name,
			tokenFunc: func(t *testing.T, env *framework.Env) string {
				return "invalid.token.value"
			},
			expectHTTP: http.StatusUnauthorized,
			expectCode: code.ErrTokenInvalid,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			token := ""
			if tc.tokenFunc != nil {
				token = tc.tokenFunc(t, env)
			}

			start := time.Now()
			resp, err := env.GetUser(token, tc.target)
			duration := time.Since(start)
			if err != nil {
				t.Fatalf("login before change failed: %v", fmt.Errorf("get user request: %w", err))
			}

			if resp.HTTPStatus() != tc.expectHTTP {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected http=%d code=%d", resp.HTTPStatus(), resp.Code))
			}
			if resp.Code != tc.expectCode {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected code=%d message=%s", resp.Code, resp.Message))
			}

			if tc.validate != nil {
				tc.validate(t, resp)
			}

			checks := map[string]bool{"response": true}
			recorder.AddCase(framework.CaseResult{
				Name:        tc.name,
				Description: tc.description,
				Success:     true,
				HTTPStatus:  resp.HTTPStatus(),
				Code:        resp.Code,
				Message:     resp.Message,
				DurationMS:  duration.Milliseconds(),
				Checks:      checks,
				Notes:       []string{tc.description},
			})
		})
	}
}

// Performance tests now live in get_performance_test.go
