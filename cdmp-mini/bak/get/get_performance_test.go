package get

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

func TestGetPerformance(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "get")
	defer recorder.Flush(t)

	const (
		password   = "InitPassw0rd!"
		workers    = 5
		iterations = 5
	)

	spec := env.NewUserSpec("getperf_", password)
	env.CreateUserAndWait(t, spec, 5*time.Second)
	defer env.ForceDeleteUserIgnore(spec.Name)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		perf perfStats
	)

	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				resp, err := env.GetUser(env.AdminToken, spec.Name)
				if err != nil || resp.HTTPStatus() != http.StatusOK || resp.Code != code.ErrSuccess {
					mu.Lock()
					perf.failure++
					mu.Unlock()
					continue
				}
				mu.Lock()
				perf.success++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	perf.duration = time.Since(start)

	total := workers * iterations
	recorder.AddPerformance(framework.PerformancePoint{
		Scenario:    "get_parallel",
		Requests:    total,
		SuccessRate: float64(perf.success) / float64(total),
		DurationMS:  perf.duration.Milliseconds(),
		QPS:         float64(total) / perf.duration.Seconds(),
		ErrorCount:  perf.failure,
	})
}
