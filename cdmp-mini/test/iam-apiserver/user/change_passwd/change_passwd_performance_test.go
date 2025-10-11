package changepasswd

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

type perfAccumulator struct {
	success  int
	failure  int
	duration time.Duration
}

func TestChangePasswordPerformance(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "change_password")
	defer recorder.Flush(t)

	const (
		workers    = 5
		iterations = 3
		basePass   = "InitPassw0rd!"
	)

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		acc perfAccumulator
	)

	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				spec := env.NewUserSpec(fmt.Sprintf("cpperf_%d_", worker), basePass)
				if resp, err := env.CreateUser(spec); err != nil || resp.HTTPStatus() != http.StatusCreated {
					mu.Lock()
					acc.failure++
					mu.Unlock()
					continue
				}
				defer env.ForceDeleteUserIgnore(spec.Name)

				if err := env.WaitForUser(spec.Name, 5*time.Second); err != nil {
					mu.Lock()
					acc.failure++
					mu.Unlock()
					continue
				}

				tokens, _, err := env.Login(spec.Name, basePass)
				if err != nil {
					mu.Lock()
					acc.failure++
					mu.Unlock()
					continue
				}

				newPassword := fmt.Sprintf("Xx1!%06d", time.Now().UnixNano()%1000000)
				resp, err := env.ChangePassword(tokens.AccessToken, spec.Name, basePass, newPassword)
				if err != nil || resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					mu.Lock()
					acc.failure++
					mu.Unlock()
					continue
				}

				if _, _, err := env.Login(spec.Name, newPassword); err != nil {
					mu.Lock()
					acc.failure++
					mu.Unlock()
					continue
				}

				mu.Lock()
				acc.success++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	acc.duration = time.Since(start)

	total := workers * iterations
	recorder.AddPerformance(framework.PerformancePoint{
		Scenario:    "change_password_parallel",
		Requests:    total,
		SuccessRate: float64(acc.success) / float64(total),
		DurationMS:  acc.duration.Milliseconds(),
		QPS:         float64(total) / acc.duration.Seconds(),
		ErrorCount:  acc.failure,
	})
}
