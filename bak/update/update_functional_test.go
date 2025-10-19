package update

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

const testDir = "test/iam-apiserver/user/update"

func TestMain(m *testing.M) {
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] export IAM_APISERVER_E2E=1 to run user update tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type updateCase struct {
	name        string
	description string
	setup       func(t *testing.T, env *framework.Env) (framework.UserSpec, bool)
	payload     func(spec framework.UserSpec) map[string]any
	expectHTTP  int
	expectCode  int
	verify      func(t *testing.T, env *framework.Env, spec framework.UserSpec, payload map[string]any, resp *framework.APIResponse)
}

func TestUpdateFunctional(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "update")
	defer recorder.Flush(t)

	const basePassword = "InitPassw0rd!"

	cases := []updateCase{
		{
			name:        "update_success",
			description: "管理员更新昵称和邮箱成功",
			setup: func(t *testing.T, env *framework.Env) (framework.UserSpec, bool) {
				spec := env.NewUserSpec("update_ok_", basePassword)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				return spec, true
			},
			payload: func(spec framework.UserSpec) map[string]any {
				return map[string]any{
					"metadata": map[string]string{"name": spec.Name},
					"nickname": "集成测试更新",
					"email":    fmt.Sprintf("%s-updated@example.com", spec.Name),
					"phone":    spec.Phone,
					"status":   1,
					"isAdmin":  0,
				}
			},
			expectHTTP: http.StatusOK,
			expectCode: code.ErrSuccess,
			verify: func(t *testing.T, env *framework.Env, spec framework.UserSpec, payload map[string]any, resp *framework.APIResponse) {
				var data struct {
					UpdateUser struct {
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
						Nickname string `json:"nickname"`
						Email    string `json:"email"`
						Phone    string `json:"phone"`
						Status   int    `json:"status"`
						IsAdmin  int    `json:"isAdmin"`
					} `json:"update_user"`
				}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("decode response data: %w", err))
				}
				if data.UpdateUser.Metadata.Name != spec.Name {
					t.Fatalf("login before change failed: %v", fmt.Errorf("metadata.name mismatch"))
				}
				if want := payload["nickname"].(string); data.UpdateUser.Nickname != want {
					t.Fatalf("login before change failed: %v", fmt.Errorf("nickname mismatch want=%s got=%s", want, data.UpdateUser.Nickname))
				}
				if want := payload["email"].(string); data.UpdateUser.Email != want {
					t.Fatalf("login before change failed: %v", fmt.Errorf("email mismatch want=%s got=%s", want, data.UpdateUser.Email))
				}
			},
		},
		{
			name:        "user_not_found",
			description: "更新不存在的用户应返回404",
			setup: func(t *testing.T, env *framework.Env) (framework.UserSpec, bool) {
				spec := env.NewUserSpec("update_missing_", basePassword)
				return spec, false
			},
			payload: func(spec framework.UserSpec) map[string]any {
				return map[string]any{
					"metadata": map[string]string{"name": spec.Name},
					"nickname": "missing user",
				}
			},
			expectHTTP: http.StatusNotFound,
			expectCode: code.ErrUserNotFound,
		},
		{
			name:        "invalid_status",
			description: "非法状态值返回校验错误",
			setup: func(t *testing.T, env *framework.Env) (framework.UserSpec, bool) {
				spec := env.NewUserSpec("update_badstatus_", basePassword)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				return spec, true
			},
			payload: func(spec framework.UserSpec) map[string]any {
				return map[string]any{
					"metadata": map[string]string{"name": spec.Name},
					"status":   2,
				}
			},
			expectHTTP: http.StatusUnprocessableEntity,
			expectCode: code.ErrValidation,
		},
	}

	var toCleanup []string
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec, created := tc.setup(t, env)
			if created {
				toCleanup = append(toCleanup, spec.Name)
			}

			payload := tc.payload(spec)
			if _, ok := payload["metadata"]; !ok && spec.Name != "" {
				payload["metadata"] = map[string]string{"name": spec.Name}
			}

			start := time.Now()
			resp, err := env.AdminRequest(http.MethodPut, fmt.Sprintf("/v1/users/%s", spec.Name), payload)
			duration := time.Since(start)
			if err != nil {
				t.Fatalf("login before change failed: %v", fmt.Errorf("update user request: %w", err))
			}

			if resp.HTTPStatus() != tc.expectHTTP {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected http=%d", resp.HTTPStatus()))
			}
			if resp.Code != tc.expectCode {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected code=%d message=%s", resp.Code, resp.Message))
			}

			if tc.verify != nil {
				tc.verify(t, env, spec, payload, resp)
			}

			checks := map[string]bool{"response": true}
			if tc.verify != nil {
				checks["payload_applied"] = true
			}

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

	for _, name := range toCleanup {
		env.ForceDeleteUserIgnore(name)
	}
}
