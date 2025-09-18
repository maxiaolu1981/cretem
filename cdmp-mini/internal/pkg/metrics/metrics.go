package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 生产者指标
	ProducerAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_producer_attempts_total",
		Help: "Total number of Kafka message sending attempts",
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

	// 消费者指标
	ConsumerMessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_messages_received_total",
		Help: "Total number of messages received by consumer",
	}, []string{"topic", "group"})

	ConsumerMessagesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_messages_processed_total",
		Help: "Total number of messages successfully processed",
	}, []string{"topic", "group"})

	ConsumerProcessingErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_processing_errors_total",
		Help: "Total number of message processing errors",
	}, []string{"topic", "group", "error_type"})
)
