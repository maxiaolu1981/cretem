package metrics

import (
	"time"

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

	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
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

)

var (
	// 布隆过滤器指标
	BloomFilterChecks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bloom_filter_checks_total",
		Help: "Total number of bloom filter checks",
	}, []string{"filter_type", "result"}) // result: hit, miss, error

	BloomFilterProcessingTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "bloom_filter_processing_seconds",
		Help:    "Time taken for bloom filter operations",
		Buckets: []float64{0.000001, 0.000005, 0.00001, 0.00005, 0.0001, 0.0005}, // 微秒到毫秒级
	}, []string{"operation"}) // operation: test, add, update

	BloomFilterCapacity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_capacity",
		Help: "Capacity of bloom filters",
	}, []string{"filter_type"})

	BloomFilterEstimatedSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_estimated_size",
		Help: "Estimated number of elements in bloom filters",
	}, []string{"filter_type"})

	BloomFilterMemoryUsage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_memory_bytes",
		Help: "Memory usage of bloom filters in bytes",
	}, []string{"filter_type"})

	BloomFilterFalsePositiveRate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_false_positive_rate",
		Help: "Configured false positive rate of bloom filters",
	}, []string{"filter_type"})

	BloomFilterUpdateOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bloom_filter_updates_total",
		Help: "Total number of bloom filter update operations",
	}, []string{"filter_type", "status"}) // status: success, failure

	BloomFilterUpdateDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "bloom_filter_update_duration_seconds",
		Help:    "Time taken for bloom filter update operations",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"filter_type"})

	BloomFilterAutoUpdateEnabled = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_auto_update_enabled",
		Help: "Whether auto update is enabled for bloom filter (1=enabled, 0=disabled)",
	}, []string{"filter_type"})

	BloomFilterStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bloom_filter_status",
		Help: "Bloom filter status (1=healthy, 0=unavailable)",
	}, []string{"filter_type"})

	  // 布隆过滤器防护统计
    BloomFilterPreventions = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "bloom_filter_preventions_total",
        Help: "Total number of requests prevented by bloom filter",
    }, []string{"filter_type"})

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
func RecordHTTPRequest(path, method, status string, duration float64, requestSize, responseSize int64) {
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

// 记录布隆过滤器检查
func RecordBloomFilterCheck(filterType string, result string, duration time.Duration) {
	BloomFilterChecks.WithLabelValues(filterType, result).Inc()
	BloomFilterProcessingTime.WithLabelValues("test").Observe(duration.Seconds())
}

// 记录布隆过滤器添加操作
func RecordBloomFilterAdd(filterType string, duration time.Duration) {
	BloomFilterProcessingTime.WithLabelValues("add").Observe(duration.Seconds())
}

// 记录布隆过滤器更新操作
func RecordBloomFilterUpdate(filterType string, success bool, duration time.Duration, itemsCount int) {
	status := "success"
	if !success {
		status = "failure"
	}

	BloomFilterUpdateOperations.WithLabelValues(filterType, status).Inc()
	BloomFilterUpdateDuration.WithLabelValues(filterType).Observe(duration.Seconds())

	if success {
		BloomFilterEstimatedSize.WithLabelValues(filterType).Set(float64(itemsCount))
	}
}

// 设置布隆过滤器容量信息
func SetBloomFilterCapacity(filterType string, capacity uint, memoryUsage uint64, falsePositiveRate float64) {
	BloomFilterCapacity.WithLabelValues(filterType).Set(float64(capacity))
	BloomFilterMemoryUsage.WithLabelValues(filterType).Set(float64(memoryUsage))
	BloomFilterFalsePositiveRate.WithLabelValues(filterType).Set(falsePositiveRate)
}

// 设置布隆过滤器状态
func SetBloomFilterStatus(filterType string, available bool) {
	status := 0.0
	if available {
		status = 1.0
	}
	BloomFilterStatus.WithLabelValues(filterType).Set(status)
}

// 设置自动更新状态
func SetBloomFilterAutoUpdateStatus(filterType string, enabled bool) {
	status := 0.0
	if enabled {
		status = 1.0
	}
	BloomFilterAutoUpdateEnabled.WithLabelValues(filterType).Set(status)
}

// 获取错误类型分类
func getBloomFilterErrorType(err error) string {
	if err == nil {
		return "none"
	}

	switch {
	case err.Error() == "bloom filter not initialized":
		return "not_initialized"
	case err.Error() == "filter type not found":
		return "filter_not_found"
	case err.Error() == "database connection error":
		return "database_error"
	case err.Error() == "context deadline exceeded":
		return "timeout"
	default:
		return "unknown"
	}
}

func CalculateMemoryUsage(capacity uint) uint64 {
	// 布隆过滤器内存占用 = 容量(bits) / 8 (转换为字节) + 数据结构开销
	baseMemory := uint64(capacity) / 8

	// 加上Bloom过滤器数据结构的基础开销（约1KB）
	const baseOverhead = 1024

	return baseMemory + baseOverhead
}
