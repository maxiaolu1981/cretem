package changepasswd

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

const testDir = "/home/mxl/cretem/cretem/cdmp-mini/test/iam-apiserver/user/change_passwd"

func TestMain(m *testing.M) {
	if os.Getenv("IAM_APISERVER_E2E") == "" {
		fmt.Println("[skip] export IAM_APISERVER_E2E=1 to enable change-password e2e tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type functionalCase struct {
	name               string
	note               string
	oldPassword        string
	newPassword        string
	overridePayload    map[string]string
	expectHTTP         int
	expectCode         int
	expectLoginNew     bool
	expectLoginOld     bool
	expectTokenInvalid bool
}

func TestChangePasswordFunctional(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "change_password")
	defer recorder.Flush(t)

	const initialPassword = "InitPassw0rd!"

	cases := []functionalCase{
		{
			name:               "happy_path",
			note:               "正常改密应刷新口令并吊销旧AT/RT",
			newPassword:        "NewPassw0rd!",
			expectHTTP:         http.StatusOK,
			expectCode:         code.ErrSuccess,
			expectLoginNew:     true,
			expectLoginOld:     false,
			expectTokenInvalid: true,
		},
		{
			name:               "weak_password",
			note:               "密码复杂度不足应拒绝",
			newPassword:        "abc12345",
			expectHTTP:         http.StatusBadRequest,
			expectCode:         code.ErrInvalidParameter,
			expectLoginNew:     false,
			expectLoginOld:     true,
			expectTokenInvalid: false,
		},
		{
			name:               "same_password",
			note:               "新旧密码相同直接拒绝",
			newPassword:        initialPassword,
			expectHTTP:         http.StatusBadRequest,
			expectCode:         code.ErrInvalidParameter,
			expectLoginNew:     false,
			expectLoginOld:     true,
			expectTokenInvalid: false,
		},
		{
			name:               "wrong_old_password",
			note:               "旧密码不正确，保持原凭证生效",
			oldPassword:        "WrongPass@123",
			newPassword:        "ValidPassw0rd!",
			expectHTTP:         http.StatusUnauthorized,
			expectCode:         code.ErrPasswordIncorrect,
			expectLoginNew:     false,
			expectLoginOld:     true,
			expectTokenInvalid: false,
		},
		{
			name:            "missing_new_password",
			note:            "缺少新密码字段应返回参数错误",
			overridePayload: map[string]string{"oldPassword": initialPassword},
			expectHTTP:      http.StatusBadRequest,
			expectCode:      code.ErrInvalidParameter,
			expectLoginNew:  false,
			expectLoginOld:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec := env.NewUserSpec("cp_", initialPassword)
			env.CreateUserAndWait(t, spec, 5*time.Second)
			defer env.ForceDeleteUserIgnore(spec.Name)

			tokens, _, err := env.Login(spec.Name, initialPassword)
			if err != nil {
				t.Fatalf("login before change failed: %v", fmt.Errorf("initial login: %w", err))
			}

			oldPassword := tc.oldPassword
			if oldPassword == "" {
				oldPassword = initialPassword
			}
			newPassword := tc.newPassword
			if newPassword == "" {
				newPassword = fmt.Sprintf("New%06d!", time.Now().UnixNano()%1000000)
			}

			payload := map[string]string{
				"oldPassword": oldPassword,
				"newPassword": newPassword,
			}
			if tc.overridePayload != nil {
				payload = tc.overridePayload
			}

			start := time.Now()
			resp, err := env.ChangePassword(tokens.AccessToken, spec.Name, payload["oldPassword"], payload["newPassword"])
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("login before change failed: %v", fmt.Errorf("change password request: %w", err))
			}

			if resp.HTTPStatus() != tc.expectHTTP {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected http=%d", resp.HTTPStatus()))
			}
			if resp.Code != tc.expectCode {
				t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected code=%d message=%s", resp.Code, resp.Message))
			}

			checks := map[string]bool{"response": true}
			if tc.expectTokenInvalid {
				checkResp, err := env.GetUser(tokens.AccessToken, spec.Name)
				if err == nil && checkResp.HTTPStatus() == http.StatusOK {
					t.Fatalf("login before change failed: %v", fmt.Errorf("old token still valid"))
				}
				checks["token_invalidated"] = true
			}

			if tc.expectLoginNew {
				if _, _, err := env.Login(spec.Name, newPassword); err != nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("login new password: %w", err))
				}
				checks["login_new_password"] = true
			}

			if tc.expectLoginOld {
				if _, _, err := env.Login(spec.Name, oldPassword); err != nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("old password login expected success: %w", err))
				}
				checks["login_old_password"] = true
			} else {
				if _, _, err := env.Login(spec.Name, oldPassword); err == nil {
					t.Fatalf("login before change failed: %v", fmt.Errorf("old password still accepted"))
				}
				checks["old_password_rejected"] = true
			}

			recorder.AddCase(framework.CaseResult{
				Name:        tc.name,
				Description: tc.note,
				Success:     true,
				HTTPStatus:  resp.HTTPStatus(),
				Code:        resp.Code,
				Message:     resp.Message,
				DurationMS:  elapsed.Milliseconds(),
				Checks:      checks,
				Notes:       []string{tc.note},
			})
		})
	}
}
