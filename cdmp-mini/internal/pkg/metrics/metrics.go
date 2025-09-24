/*
这份 Go 代码基于 prometheus/client_golang 库，定义了Kafka（生产者 / 消费者）、数据库、HTTP、缓存, redis五大核心场景的监控指标（Counter/Gauge/Histogram），并提供了 7 个辅助函数用于简化业务代码中指标的上报操作。
所有指标通过 promauto.NewXXX 自动注册到 Prometheus 默认注册表，无需手动调用 prometheus.Register；函数的核心作用是封装指标的标签赋值、计数 / 观测逻辑，让业务代码无需关注 Prometheus 指标的底层操作，只需传入业务参数即可完成监控上报。
*/
package metrics

import (
	"context"

	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
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

// Redis操作指标
var (
	RedisOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_operations_total",
		Help: "Redis操作总数",
	}, []string{"operation", "status"}) // operation: get, set, del; status: success, error

	RedisOperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "redis_operation_duration_seconds",
		Help:    "Redis操作延迟",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	}, []string{"operation"})

	RedisErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_errors_total",
		Help: "Redis错误总数",
	}, []string{"operation", "error_type"}) // error_type: connection, timeout, serialization等

	RedisCacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "redis_cache_size_bytes",
		Help: "Redis缓存大小",
	}, []string{"key_pattern"})
)

var (
	// HTTP指标 - 使用 promauto 自动注册
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

	// 新增的HTTP增强指标 - 使用 promauto 自动注册
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP请求总数",
	}, []string{"path", "method", "status", "error_type", "user_id", "tenant_id", "client_ip", "user_agent", "host"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP请求延迟分布",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
	}, []string{"path", "method", "status", "error_type"})

	HTTPRequestsInProgress = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_requests_in_progress",
		Help: "当前正在处理的HTTP请求数",
	}, []string{"path", "method"})

	CacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_cache_requests_total",
		Help: "HTTP请求缓存命中统计",
	}, []string{"path", "cache_status", "user_id", "tenant_id"})

	SlowHTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_slow_requests_total",
		Help: "慢HTTP请求统计（>1s）",
	}, []string{"path", "method", "status", "error_type"})

	// 原有的HTTPErrors指标保持不变
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

// 统一处理数据库查询的监控上报，包括查询错误计数和查询耗时统计
func RecordDatabaseQuery(operation, table string, duration float64, err error) {
	DatabaseQueryDuration.WithLabelValues(operation, table).Observe(duration)
	if err != nil {
		errorType := GetDatabaseErrorType(err)
		DatabaseQueryErrors.WithLabelValues(operation, table, errorType).Inc()
	}
}

// GetDatabaseErrorType 数据库错误分类，仅用于监控指标
func GetDatabaseErrorType(err error) string {
	if err == nil {
		return "success"
	}

	// 1. 先检查是否是业务框架已知错误
	coder := errors.ParseCoderByErr(err)
	if coder != nil {
		// 对于已知业务错误，返回通用分类
		return "business_error"
	}

	// 2. 检查上下文相关错误
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}

	// 3. 检查GORM特定错误
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "not_found"
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return "duplicate_key"
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return "foreign_key_violation"
	}

	// 4. 检查MySQL驱动错误
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1213: // ER_LOCK_DEADLOCK
			return "deadlock"
		case 1205: // ER_LOCK_WAIT_TIMEOUT
			return "lock_timeout"
		case 1062: // ER_DUP_ENTRY
			return "duplicate_entry"
		case 1452: // ER_NO_REFERENCED_ROW
			return "foreign_key_violation"
		case 1045: // ER_ACCESS_DENIED_ERROR
			return "access_denied"
		case 2002, 2003: // 连接相关错误
			return "connection_error"
		default:
			return "mysql_error"
		}
	}

	// 5. 基于错误消息的模式匹配
	errMsg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errMsg, "timeout"):
		return "timeout"
	case strings.Contains(errMsg, "connection"):
		return "connection_error"
	case strings.Contains(errMsg, "deadlock"):
		return "deadlock"
	case strings.Contains(errMsg, "duplicate"):
		return "duplicate"
	case strings.Contains(errMsg, "constraint"):
		return "constraint_violation"
	case strings.Contains(errMsg, "full"):
		return "disk_full"
	default:
		return "unknown"
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

// RecordHTTPRequest 记录HTTP请求指标（简化版，与您现有代码兼容）
func RecordHTTPRequest(path, method, status string, duration float64, requestSize, responseSize int64,
	clientIP, userAgent, host, errorCode, errorType, userID, tenantID string) {

	// 使用现有的HTTP指标
	HTTPResponseTime.WithLabelValues(path, method, status).Observe(duration)

	if requestSize > 0 {
		HTTPRequestSize.WithLabelValues(path, method).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		HTTPResponseSize.WithLabelValues(path, method, status).Observe(float64(responseSize))
	}

	// 记录新增的增强版指标
	HTTPRequestsTotal.WithLabelValues(
		path, method, status, errorType, userID, tenantID, clientIP, userAgent, host,
	).Inc()

	// 记录延迟分布
	HTTPRequestDuration.WithLabelValues(
		path, method, status, errorType,
	).Observe(duration)

	// 记录慢请求
	if duration > 1.0 {
		SlowHTTPRequests.WithLabelValues(path, method, status, errorType).Inc()
	}
}

// GetRedisErrorType Redis错误分类
func GetRedisErrorType(err error) string {
	if err == nil {
		return "success"
	}

	errMsg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errMsg, "timeout"):
		return "timeout"
	case strings.Contains(errMsg, "connection"):
		return "connection_error"
	case strings.Contains(errMsg, "network"):
		return "network_error"
	case strings.Contains(errMsg, "max memory"):
		return "memory_limit"
	case strings.Contains(errMsg, "serialize"), strings.Contains(errMsg, "marshal"):
		return "serialization_error"
	case strings.Contains(errMsg, "deserialize"), strings.Contains(errMsg, "unmarshal"):
		return "deserialization_error"
	case strings.Contains(errMsg, "nil"): // redis.Nil错误
		return "key_not_found"
	default:
		return "unknown_error"
	}
}
