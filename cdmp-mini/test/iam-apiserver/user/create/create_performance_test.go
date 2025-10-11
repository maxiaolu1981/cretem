package create

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/test/iam-apiserver/tools/framework"
)

type perfSummary struct {
	success  int
	failure  int
	duration time.Duration
}

func TestCreatePerformance(t *testing.T) {
	env := framework.NewEnv(t)
	outputDir := env.EnsureOutputDir(t, testDir)
	recorder := framework.NewRecorder(t, outputDir, "create")
	defer recorder.Flush(t)

	const (
		workers    = 5
		iterations = 3
		password   = "InitPassw0rd!"
	)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		perf    perfSummary
		created = make([]string, 0, workers*iterations)
	)
	defer func() {
		for _, name := range created {
			env.ForceDeleteUserIgnore(name)
		}
	}()

	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				spec := env.NewUserSpec(fmt.Sprintf("createperf_%d_", worker), password)
				resp, err := env.CreateUser(spec)
				if err != nil || resp.HTTPStatus() != http.StatusCreated {
					mu.Lock()
					perf.failure++
					mu.Unlock()
					continue
				}
				if err := env.WaitForUser(spec.Name, 5*time.Second); err != nil {
					mu.Lock()
					perf.failure++
					mu.Unlock()
					continue
				}
				mu.Lock()
				perf.success++
				created = append(created, spec.Name)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	perf.duration = time.Since(start)

	total := workers * iterations
	recorder.AddPerformance(framework.PerformancePoint{
		Scenario:    "create_parallel",
		Requests:    total,
		SuccessRate: float64(perf.success) / float64(total),
		DurationMS:  perf.duration.Milliseconds(),
		QPS:         float64(total) / perf.duration.Seconds(),
		ErrorCount:  perf.failure,
	})
}
