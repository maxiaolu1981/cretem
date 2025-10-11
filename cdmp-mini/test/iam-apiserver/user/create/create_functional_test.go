package create


import (
    "fmt"
    "net/http"
    "os"
    "testing"
    "time"

    "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
    "github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

const testDir = "test/iam-apiserver/user/create"

func TestMain(m *testing.M) {
    if os.Getenv("IAM_APISERVER_E2E") == "" {
        fmt.Println("[skip] export IAM_APISERVER_E2E=1 to run user create tests")
        os.Exit(0)
    }
    os.Exit(m.Run())
}

type createCase struct {
    name        string
    description string
    specFunc    func(env *framework.Env) framework.UserSpec
    payload     map[string]any
    expectHTTP  int
    expectCode  int
    expectError bool
}

func TestCreateFunctional(t *testing.T) {
    env := framework.NewEnv(t)
    outputDir := env.EnsureOutputDir(t, testDir)
    recorder := framework.NewRecorder(t, outputDir, "create")
    defer recorder.Flush(t)

    basePassword := "InitPassw0rd!"

    successSpec := env.NewUserSpec("create_", basePassword)

    cases := []createCase{
        {
            name:        "create_success",
            description: "成功创建用户并可查询",
            specFunc: func(env *framework.Env) framework.UserSpec {
                return env.NewUserSpec("create_ok_", basePassword)
            },
            expectHTTP: http.StatusCreated,
            expectCode: code.ErrSuccess,
        },
        {
            name:        "duplicate_user",
            description: "重复创建应返回已存在错误",
            specFunc: func(env *framework.Env) framework.UserSpec {
                env.CreateUserAndWait(t, successSpec, 5*time.Second)
                return successSpec
            },
            expectHTTP:  http.StatusConflict,
            expectCode:  code.ErrUserAlreadyExist,
            expectError: true,
        },
        {
            name:        "invalid_payload",
            description: "缺少 metadata 应返回参数错误",
            specFunc: func(env *framework.Env) framework.UserSpec {
                spec := env.NewUserSpec("create_bad_", basePassword)
                return framework.UserSpec{Name: spec.Name}
            },
            payload: map[string]any{
                "nickname": "缺少metadata",
            },
            expectHTTP:  http.StatusBadRequest,
            expectCode:  code.ErrInvalidParameter,
            expectError: true,
        },
    }

    createdUsers := make(map[string]struct{})
    defer func() {
        for u := range createdUsers {
            env.ForceDeleteUserIgnore(u)
        }
    }()

    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            spec := tc.specFunc(env)
            payload := tc.payload
            if payload == nil {
                payload = map[string]any{
                    "metadata": map[string]string{"name": spec.Name},
                    "nickname": spec.Nickname,
                    "password": spec.Password,
                    "email":    spec.Email,
                    "phone":    spec.Phone,
                    "status":   spec.Status,
                    "isAdmin":  spec.IsAdmin,
                }
            } else if _, ok := payload["metadata"]; !ok && spec.Name != "" {
                payload["metadata"] = map[string]string{"name": spec.Name}
            }

            start := time.Now()
            resp, err := env.AdminRequest(http.MethodPost, "/v1/users", payload)
            duration := time.Since(start)
            if err != nil {
                t.Fatalf("login before change failed: %v", fmt.Errorf("create user request: %w", err))
            }

            if resp.HTTPStatus() != tc.expectHTTP {
                t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected http=%d code=%d", resp.HTTPStatus(), resp.Code))
            }
            if resp.Code != tc.expectCode {
                t.Fatalf("login before change failed: %v", fmt.Errorf("unexpected code=%d message=%s", resp.Code, resp.Message))
            }

            checks := map[string]bool{"response": true}
            if resp.HTTPStatus() == http.StatusCreated {
                createdUsers[spec.Name] = struct{}{}
                if err := env.WaitForUser(spec.Name, 5*time.Second); err != nil {
                    t.Fatalf("login before change failed: %v", fmt.Errorf("wait for user ready: %w", err))
                }
                checks["user_persisted"] = true
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
