package performance

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

const testDir = "test/iam-apiserver/user/delete-force"

func TestMain(m *testing.M) {
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] export IAM_APISERVER_E2E=1 to enable delete-force e2e tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type deleteForceCase struct {
	name        string
	description string
	setup       func(t *testing.T, env *framework.Env) (username string, needsCleanup bool)
	expectHTTP  int
	expectCode  int
	postCheck   func(t *testing.T, env *framework.Env, username string)
}

func TestDeleteForceFunctional(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "delete_force")
	defer recorder.Flush(t)

	const basePassword = "InitPassw0rd!"

	cases := []deleteForceCase{
		{
			name:        "delete_existing_user",
			description: "管理员可成功删除现有用户并返回成功码",
			setup: func(t *testing.T, env *framework.Env) (string, bool) {
				spec := env.NewUserSpec("del_force_", basePassword)
				env.CreateUserAndWait(t, spec, 5*time.Second)
				return spec.Name, true
			},
			expectHTTP: http.StatusOK,
			expectCode: code.ErrSuccess,
			postCheck: func(t *testing.T, env *framework.Env, username string) {
				t.Helper()
				resp, err := env.AdminRequest(http.MethodGet, fmt.Sprintf("/v1/users/%s", username), nil)
				if err != nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("get user after force delete: %w", err))
				}
				if resp.HTTPStatus() != http.StatusNotFound {
					t.Fatalf("login before change failed: %v", fmt.Errorf("expected get http=404 got=%d", resp.HTTPStatus()))
				}
				if resp.Code != code.ErrUserNotFound {
					t.Fatalf("login before change failed: %v", fmt.Errorf("expected code=%d got=%d", code.ErrUserNotFound, resp.Code))
				}
			},
		},
		{
			name:        "delete_nonexistent_user",
			description: "删除不存在的用户应返回404",
			setup: func(t *testing.T, env *framework.Env) (string, bool) {
				username := fmt.Sprintf("missing_%d", time.Now().UnixNano())
				return username, false
			},
			expectHTTP: http.StatusNotFound,
			expectCode: code.ErrUserNotFound,
		},
		{
			name:        "invalid_username",
			description: "非法用户名应触发参数错误",
			setup: func(t *testing.T, env *framework.Env) (string, bool) {
				return "bad_user!", false
			},
			expectHTTP: http.StatusBadRequest,
			expectCode: code.ErrInvalidParameter,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			username, needsCleanup := tc.setup(t, env)
			cleanupName := ""
			if needsCleanup {
				cleanupName = username
				defer func() {
					if cleanupName != "" {
						env.ForceDeleteUserIgnore(cleanupName)
					}
				}()
			}

			start := time.Now()
			resp, err := env.ForceDeleteUser(username)
			duration := time.Since(start)
			if err != nil {
				t.Fatalf("login before change failed: %v", fmt.Errorf("force delete user request: %w", err))
			}

			status := resp.HTTPStatus()
			if status != tc.expectHTTP {
				if !(tc.expectHTTP == http.StatusOK && status == http.StatusNoContent) {
					t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected http=%d", status))
				}
			}

			if resp.Code != tc.expectCode {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected code=%d message=%s", resp.Code, resp.Message))
			}

			checks := map[string]bool{"response": true}
			if tc.postCheck != nil {
				tc.postCheck(t, env, username)
				checks["post_validation"] = true
			}

			if needsCleanup && resp.HTTPStatus() == http.StatusOK {
				cleanupName = ""
			}
			if needsCleanup && resp.HTTPStatus() == http.StatusNoContent {
				cleanupName = ""
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
}
