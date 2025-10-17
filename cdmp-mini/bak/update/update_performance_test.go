package update

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

type perfStats struct {
	success  int
	failure  int
	duration time.Duration
}

func TestUpdatePerformance(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "update")
	defer recorder.Flush(t)

	const (
		workers    = 4
		iterations = 3
		password   = "InitPassw0rd!"
	)

	specs := make([]framework.UserSpec, workers)
	for i := range specs {
		specs[i] = env.NewUserSpec(fmt.Sprintf("updateperf_%d_", i), password)
		env.CreateUserAndWait(t, specs[i], 5*time.Second)
	}
	defer func() {
		for _, spec := range specs {
			env.ForceDeleteUserIgnore(spec.Name)
		}
	}()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		perf perfStats
	)

	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			spec := specs[idx]
			for j := 0; j < iterations; j++ {
				payload := map[string]any{
					"metadata": map[string]string{"name": spec.Name},
					"nickname": fmt.Sprintf("perf_%d_%d", idx, j),
					"email":    fmt.Sprintf("%s-%d@example.com", spec.Name, j),
				}
				resp, err := env.AdminRequest(http.MethodPut, fmt.Sprintf("/v1/users/%s", spec.Name), payload)
				if err != nil || resp.HTTPStatus() != http.StatusOK {
					mu.Lock()
					perf.failure++
					mu.Unlock()
					continue
				}
				mu.Lock()
				perf.success++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	perf.duration = time.Since(start)

	total := workers * iterations
	recorder.AddPerformance(framework.PerformancePoint{
		Scenario:    "update_parallel",
		Requests:    total,
		SuccessRate: float64(perf.success) / float64(total),
		DurationMS:  perf.duration.Milliseconds(),
		QPS:         float64(total) / perf.duration.Seconds(),
		ErrorCount:  perf.failure,
	})
}
