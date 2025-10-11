package login

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

type perfStats struct {
	success  int
	failure  int
	duration time.Duration
}

func TestLoginPerformance(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "login")
	defer recorder.Flush(t)

	baseSpec := env.NewUserSpec("loginperf_", "InitPassw0rd!")
	env.CreateUserAndWait(t, baseSpec, 5*time.Second)
	defer env.ForceDeleteUserIgnore(baseSpec.Name)

	const (
		workers    = 10
		iterations = 5
	)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		stats perfStats
	)

	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				resp, err := env.AuthorizedRequest(http.MethodPost, "/login", "", map[string]string{
					"username": baseSpec.Name,
					"password": baseSpec.Password,
				})
				if err != nil || resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					mu.Lock()
					stats.failure++
					mu.Unlock()
					continue
				}
				mu.Lock()
				stats.success++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	stats.duration = time.Since(start)

	total := workers * iterations
	recorder.AddPerformance(framework.PerformancePoint{
		Scenario:    "login_parallel",
		Requests:    total,
		SuccessRate: float64(stats.success) / float64(total),
		DurationMS:  stats.duration.Milliseconds(),
		QPS:         float64(total) / stats.duration.Seconds(),
		ErrorCount:  stats.failure,
	})
}
