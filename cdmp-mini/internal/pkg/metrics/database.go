package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 定义数据库操作指标
var (
	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_operations_total",
			Help: "Total number of database operations by type",
		},
		[]string{"operation", "table"},
	)

	DatabaseDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_operation_duration_seconds",
			Help:    "Duration of database operations",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5},
		},
		[]string{"operation", "table"},
	)

	DatabaseErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_errors_total",
			Help: "Total number of database errors",
		},
		[]string{"operation", "table", "error_type"},
	)
)

// RecordOperation 记录数据库操作
func RecordOperation(operation, table string, duration time.Duration) {
	DatabaseOperations.WithLabelValues(operation, table).Inc()
	DatabaseDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// RecordError 记录数据库错误
func RecordError(operation, table, errorType string) {
	DatabaseErrors.WithLabelValues(operation, table, errorType).Inc()
}
