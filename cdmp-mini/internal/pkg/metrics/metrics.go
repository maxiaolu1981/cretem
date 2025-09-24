package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 生产者指标
	ProducerAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_attempts_total",
		Help: "生产者发送消息的总尝试次数（包括首次发送和重试）",
	}, []string{"topic", "operation"})

	ProducerSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_success_total",
		Help: "Total number of successfully sent Kafka messages",
	}, []string{"topic", "operation"})

	ProducerFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_failures_total",
		Help: "Total number of failed Kafka message sending attempts",
	}, []string{"topic", "operation", "error_type"})

	ProducerRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_retries_total",
		Help: "Total number of message retries",
	}, []string{"topic", "operation"})

	DeadLetterMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_dead_letter_messages_total",
		Help: "Total number of messages sent to dead letter queue",
	}, []string{"topic", "operation"})

	// 消息处理延迟指标
	MessageProcessingTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kafka_message_processing_seconds",
		Help:    "Time taken to process messages",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
	}, []string{"topic", "operation", "status"})

	// 业务处理指标
	BusinessProcessingTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "business_processing_seconds",
		Help:    "Time taken for business logic processing",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	}, []string{"operation"})

	BusinessSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "business_operations_success_total",
		Help: "Total number of successful business operations",
	}, []string{"operation"})

	BusinessFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "business_operations_failures_total",
		Help: "Total number of failed business operations",
	}, []string{"operation", "error_type"})
)

var (
	// Kafka消费者指标
	ConsumerMessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_messages_received_total",
		Help: "Total number of messages received by consumer",
	}, []string{"topic", "group", "operation"})

	ConsumerMessagesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_messages_processed_total",
		Help: "Total number of messages successfully processed",
	}, []string{"topic", "group", "operation"})

	ConsumerProcessingErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_processing_errors_total",
		Help: "Total number of message processing errors",
	}, []string{"topic", "group", "operation", "error_type"})

	ConsumerProcessingTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kafka_consumer_processing_seconds",
		Help:    "Time taken to process messages by consumer",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
	}, []string{"topic", "group", "operation", "status"})

	ConsumerRetryMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_retry_messages_total",
		Help: "Total number of messages sent to retry topic",
	}, []string{"topic", "group", "operation", "error_type"})

	ConsumerDeadLetterMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_dead_letter_messages_total",
		Help: "Total number of messages sent to dead letter queue by consumer",
	}, []string{"topic", "group", "operation", "error_type"})

	ConsumerLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kafka_consumer_lag",
		Help: "Current consumer lag (estimated)",
	}, []string{"topic", "group"})

	// 数据库操作指标
	DatabaseQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "database_query_duration_seconds",
		Help:    "Time taken for database queries",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2},
	}, []string{"operation", "table"})

	DatabaseQueryErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "database_query_errors_total",
		Help: "Total number of database query errors",
	}, []string{"operation", "table", "error_type"})

	DatabaseConnectionsInUse = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "database_connections_in_use",
		Help: "Number of database connections currently in use",
	}, []string{"pool"})

	DatabaseConnectionsWait = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "database_connections_wait_total",
		Help: "Total number of database connection waits",
	}, []string{"pool"})
)

var (
	// HTTP指标
	HTTPResponseTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_time_seconds",
		Help:    "Duration of HTTP requests",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5},
	}, []string{"path", "method", "status"})

	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of ongoing HTTP requests",
	})

	HTTPRequestSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_size_bytes",
		Help:    "HTTP request size in bytes",
		Buckets: prometheus.ExponentialBuckets(100, 10, 6), // 100B to 100MB
	}, []string{"path", "method"})

	HTTPResponseSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "HTTP response size in bytes",
		Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 1GB
	}, []string{"path", "method", "status"})

	HTTPErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_errors_total",
		Help: "Total number of HTTP errors by type",
	}, []string{"method", "path", "status", "error_type"})
)

var (
	// 缓存命中指标
	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "user_cache_hits_total",
		Help: "Total user cache hits",
	}, []string{"type"}) // hit, miss, null_hit

	// 数据库查询指标
	DBQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "user_db_queries_total",
		Help: "Total user database queries",
	}, []string{"result"}) // found, not_found

	// RequestsMerged 记录被singleflight合并的请求数量
	RequestsMerged = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_service_requests_merged_total",
			Help: "被singleflight合并的请求数量",
		},
		[]string{"operation"}, // 操作类型：get, create, update等
	)

	// CacheErrors 记录缓存相关错误
	CacheErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_service_cache_errors_total",
			Help: "缓存操作错误数量",
		},
		[]string{"type", "operation"}, // 错误类型，操作类型
	)
	// 空值缓存数量（使用Gauge）
	CacheNullValuesCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cache_null_values_count",
		Help: "Current number of null value caches in Redis",
	})

	// 空值缓存操作统计
	CacheNullValueOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_null_value_operations_total",
		Help: "Total null value cache operations",
	}, []string{"operation"}) // operation: set, hit, expire, erreration: set, hit, expire
)

// 辅助函数用于记录数据库操作
func RecordDatabaseQuery(operation, table string, duration float64, err error) {

	if err != nil {

		errorType := getDatabaseErrorType(err)
		DatabaseQueryErrors.WithLabelValues(operation, table, errorType).Inc()
	}

	DatabaseQueryDuration.WithLabelValues(operation, table).Observe(duration)
}

func getDatabaseErrorType(err error) string {
	// 根据实际错误类型进行分类
	// 这里可以根据你的数据库驱动错误类型进行细化
	switch {
	case err.Error() == "context deadline exceeded":
		return "timeout"
	case err.Error() == "connection refused":
		return "connection_error"
	case err.Error() == "duplicate key":
		return "constraint_violation"
	default:
		return "unknown"
	}
}

// 辅助函数用于记录HTTP请求
func RecordHTTPRequest(path, method, status string, duration float64, requestSize, responseSize int64, clientIP string, userAgent string,
	host string, errorCode string, errorType string, userID string, tenantID string) {
	HTTPResponseTime.WithLabelValues(path, method, status).Observe(duration)
	HTTPRequestsTotal.WithLabelValues(path, method, status).Inc()

	if requestSize > 0 {
		HTTPRequestSize.WithLabelValues(path, method).Observe(float64(requestSize))
	}

	if responseSize > 0 {
		HTTPResponseSize.WithLabelValues(path, method, status).Observe(float64(responseSize))
	}
}

// HTTP中间件使用的函数
func HTTPMiddlewareStart() {
	HTTPRequestsInFlight.Inc()
}

func HTTPMiddlewareEnd() {
	HTTPRequestsInFlight.Dec()
}

// 数据库连接池监控
func SetDatabaseConnectionsInUse(poolName string, count int) {
	DatabaseConnectionsInUse.WithLabelValues(poolName).Set(float64(count))
}

func IncDatabaseConnectionsWait(poolName string) {
	DatabaseConnectionsWait.WithLabelValues(poolName).Inc()
}

// 在 metrics 包中添加以下指标定义

// HTTP请求总数（增强版）
var HTTPRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP请求总数",
	},
	[]string{"path", "method", "status", "error_type", "user_id", "tenant_id", "client_ip", "user_agent", "host"},
)

// HTTP请求延迟分布
var HTTPRequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP请求延迟分布",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
	},
	[]string{"path", "method", "status", "error_type"},
)

// 并发请求数
var HTTPRequestsInProgress = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "http_requests_in_progress",
		Help: "当前正在处理的HTTP请求数",
	},
	[]string{"path", "method"},
)

// 缓存请求统计
var CacheRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_cache_requests_total",
		Help: "HTTP请求缓存命中统计",
	},
	[]string{"path", "cache_status", "user_id", "tenant_id"},
)

// 慢请求统计
var SlowHTTPRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_slow_requests_total",
		Help: "慢HTTP请求统计（>1s）",
	},
	[]string{"path", "method", "status", "error_type"},
)

// 注册所有指标
func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		HTTPRequestsInProgress,
		CacheRequests,
		SlowHTTPRequests,
	)
}
