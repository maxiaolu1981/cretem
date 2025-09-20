package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// 配置参数
type Config struct {
	PrometheusURL string        `json:"prometheus_url"`
	Interval      time.Duration `json:"interval"`
	Topic         string        `json:"topic"`
	Groups        []string      `json:"groups"`
	Operation     string        `json:"operation"`
	Table         string        `json:"table"`
}

// 消费者组指标
type ConsumerGroupMetrics struct {
	Group             string  `json:"group"`
	Received          float64 `json:"received"`
	Processed         float64 `json:"processed"`
	Errors            float64 `json:"errors"`
	Retry             float64 `json:"retry"`
	DeadLetter        float64 `json:"dead_letter"`
	Lag               float64 `json:"lag"`
	ProcessingTime95  float64 `json:"processing_time_95"`
	ProcessingTimeAvg float64 `json:"processing_time_avg"`
	ErrorRate         float64 `json:"error_rate"`
	RetryRate         float64 `json:"retry_rate"`
	DeadLetterRate    float64 `json:"dead_letter_rate"`
	Throughput        float64 `json:"throughput"`
}

// 生产者指标
type ProducerMetrics struct {
	Attempts           float64 `json:"attempts"`
	Success            float64 `json:"success"`
	Failures           float64 `json:"failures"`
	Retries            float64 `json:"retries"`
	DeadLetterMessages float64 `json:"dead_letter_messages"`
	SuccessRate        float64 `json:"success_rate"`
	FailureRate        float64 `json:"failure_rate"`
	RetryRate          float64 `json:"retry_rate"`
	Throughput         float64 `json:"throughput"`
}

// 业务处理指标
type BusinessMetrics struct {
	Success           float64 `json:"success"`
	Failures          float64 `json:"failures"`
	ProcessingTime95  float64 `json:"processing_time_95"`
	ProcessingTimeAvg float64 `json:"processing_time_avg"`
	SuccessRate       float64 `json:"success_rate"`
	Throughput        float64 `json:"throughput"`
}

// 消息处理指标
type MessageProcessingMetrics struct {
	Time95     float64 `json:"time_95"`
	TimeAvg    float64 `json:"time_avg"`
	Count      float64 `json:"count"`
	ErrorRate  float64 `json:"error_rate"`
	Throughput float64 `json:"throughput"`
}

// 数据库指标
// 数据库指标
type DatabaseMetrics struct {
	QueryCount   float64            `json:"query_count"`
	QueryErrors  float64            `json:"query_errors"`
	QueryTime95  float64            `json:"query_time_95"`
	QueryTimeAvg float64            `json:"query_time_avg"`
	ErrorRate    float64            `json:"error_rate"`
	Throughput   float64            `json:"throughput"`
	ErrorTypes   map[string]float64 `json:"error_types"`
	Operations   map[string]float64 `json:"operations"` // 添加这个字段
}

// 完整的监控指标
type Metrics struct {
	Producer          ProducerMetrics          `json:"producer"`
	ConsumerGroups    []ConsumerGroupMetrics   `json:"consumer_groups"`
	Business          BusinessMetrics          `json:"business"`
	MessageProcessing MessageProcessingMetrics `json:"message_processing"`
	Database          DatabaseMetrics          `json:"database"`

	TotalReceived   float64 `json:"total_received"`
	TotalProcessed  float64 `json:"total_processed"`
	TotalErrors     float64 `json:"total_errors"`
	TotalRetry      float64 `json:"total_retry"`
	TotalDeadLetter float64 `json:"total_dead_letter"`
}

// Prometheus客户端
type PrometheusClient struct {
	client v1.API
}

func NewPrometheusClient(url string) (*PrometheusClient, error) {
	client, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, err
	}

	return &PrometheusClient{
		client: v1.NewAPI(client),
	}, nil
}

// 查询Prometheus
func (p *PrometheusClient) Query(query string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := p.client.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}

	if len(warnings) > 0 {
		log.Printf("Warnings: %v", warnings)
	}

	if result == nil || result.Type() != model.ValVector {
		return 0, nil
	}

	vector := result.(model.Vector)
	if len(vector) == 0 {
		return 0, nil
	}

	return float64(vector[0].Value), nil
}

// 查询速率
func (p *PrometheusClient) QueryRate(query string, duration string) (float64, error) {
	rateQuery := fmt.Sprintf("rate(%s[%s])", query, duration)
	return p.Query(rateQuery)
}

// 获取生产者指标
func (p *PrometheusClient) GetProducerMetrics(topic, operation string) (*ProducerMetrics, error) {
	var metrics ProducerMetrics
	var success, failures, retries, deadLetter float64
	var err error

	success, err = p.Query(fmt.Sprintf(`sum(kafka_producer_success_total{topic="%s",operation="%s"})`, topic, operation))
	if err != nil {
		return nil, err
	}
	metrics.Success = success

	failures, err = p.Query(fmt.Sprintf(`sum(kafka_producer_failures_total{topic="%s",operation="%s"})`, topic, operation))
	if err != nil {
		return nil, err
	}
	metrics.Failures = failures

	retries, err = p.Query(fmt.Sprintf(`sum(kafka_producer_retries_total{topic="%s",operation="%s"})`, topic, operation))
	if err != nil {
		return nil, err
	}
	metrics.Retries = retries

	deadLetter, err = p.Query(fmt.Sprintf(`sum(kafka_dead_letter_messages_total{topic="%s",operation="%s"})`, topic, operation))
	if err != nil {
		return nil, err
	}
	metrics.DeadLetterMessages = deadLetter

	metrics.Attempts = metrics.Success + metrics.Failures

	// 计算比率
	if metrics.Attempts > 0 {
		metrics.SuccessRate = (metrics.Success / metrics.Attempts) * 100
		metrics.FailureRate = (metrics.Failures / metrics.Attempts) * 100
		metrics.RetryRate = (metrics.Retries / metrics.Attempts) * 100
	}

	// 吞吐量
	metrics.Throughput, err = p.QueryRate(fmt.Sprintf(`kafka_producer_success_total{topic="%s",operation="%s"}`, topic, operation), "5m")
	if err != nil {
		return nil, err
	}

	return &metrics, nil
}

// 获取消费者组指标
func (p *PrometheusClient) GetConsumerGroupMetrics(topic, group, operation string) (*ConsumerGroupMetrics, error) {
	var metrics ConsumerGroupMetrics
	metrics.Group = group

	var received, processed, errors, retry, deadLetter, lag float64
	var err error

	received, err = p.Query(fmt.Sprintf(`sum(kafka_consumer_messages_received_total{topic="%s",group="%s",operation="%s"})`, topic, group, operation))
	if err != nil {
		return nil, err
	}
	metrics.Received = received

	processed, err = p.Query(fmt.Sprintf(`sum(kafka_consumer_messages_processed_total{topic="%s",group="%s",operation="%s"})`, topic, group, operation))
	if err != nil {
		return nil, err
	}
	metrics.Processed = processed

	errors, err = p.Query(fmt.Sprintf(`sum(kafka_consumer_processing_errors_total{topic="%s",group="%s",operation="%s"})`, topic, group, operation))
	if err != nil {
		return nil, err
	}
	metrics.Errors = errors

	retry, err = p.Query(fmt.Sprintf(`sum(kafka_consumer_retry_messages_total{topic="%s",group="%s",operation="%s"})`, topic, group, operation))
	if err != nil {
		return nil, err
	}
	metrics.Retry = retry

	deadLetter, err = p.Query(fmt.Sprintf(`sum(kafka_consumer_dead_letter_messages_total{topic="%s",group="%s",operation="%s"})`, topic, group, operation))
	if err != nil {
		return nil, err
	}
	metrics.DeadLetter = deadLetter

	lag, err = p.Query(fmt.Sprintf(`kafka_consumer_lag{topic="%s",group="%s"}`, topic, group))
	if err != nil {
		return nil, err
	}
	metrics.Lag = lag

	// 处理时间分位数
	metrics.ProcessingTime95, _ = p.Query(fmt.Sprintf(`histogram_quantile(0.95, sum(rate(kafka_consumer_processing_seconds_bucket{topic="%s",group="%s",operation="%s"}[5m])) by (le))`, topic, group, operation))
	metrics.ProcessingTimeAvg, _ = p.Query(fmt.Sprintf(`rate(kafka_consumer_processing_seconds_sum{topic="%s",group="%s",operation="%s"}[5m]) / rate(kafka_consumer_processing_seconds_count{topic="%s",group="%s",operation="%s"}[5m])`, topic, group, operation, topic, group, operation))

	// 吞吐量
	metrics.Throughput, err = p.QueryRate(fmt.Sprintf(`kafka_consumer_messages_processed_total{topic="%s",group="%s",operation="%s"}`, topic, group, operation), "5m")
	if err != nil {
		return nil, err
	}

	// 计算比率
	if metrics.Received > 0 {
		metrics.ErrorRate = (metrics.Errors / metrics.Received) * 100
		metrics.RetryRate = (metrics.Retry / metrics.Received) * 100
		metrics.DeadLetterRate = (metrics.DeadLetter / metrics.Received) * 100
	}

	return &metrics, nil
}

// 获取业务指标
// 获取业务指标（修正版）
func (p *PrometheusClient) GetBusinessMetrics(operation string) (*BusinessMetrics, error) {
	var metrics BusinessMetrics

	// 根据你的实际指标调整operation名称
	// 从 "create" 改为 "user_create"
	actualOperation := "user_create"
	if operation == "create" {
		actualOperation = "user_create"
	} else if operation == "update" {
		actualOperation = "user_update"
	}

	var success, failures float64
	var err error

	success, err = p.Query(fmt.Sprintf(`sum(business_operations_success_total{operation="%s"})`, actualOperation))
	if err != nil {
		return nil, err
	}
	metrics.Success = success

	failures, err = p.Query(fmt.Sprintf(`sum(business_operations_failures_total{operation="%s"})`, actualOperation))
	if err != nil {
		// 失败指标可能不存在，忽略错误
		metrics.Failures = 0
	} else {
		metrics.Failures = failures
	}

	// 处理时间
	metrics.ProcessingTime95, _ = p.Query(fmt.Sprintf(`histogram_quantile(0.95, sum(rate(business_processing_seconds_bucket{operation="%s"}[5m])) by (le))`, actualOperation))
	metrics.ProcessingTimeAvg, _ = p.Query(fmt.Sprintf(`rate(business_processing_seconds_sum{operation="%s"}[5m]) / rate(business_processing_seconds_count{operation="%s"}[5m])`, actualOperation, actualOperation))

	// 成功率
	total := metrics.Success + metrics.Failures
	if total > 0 {
		metrics.SuccessRate = (metrics.Success / total) * 100
	}

	// 吞吐量
	metrics.Throughput, err = p.QueryRate(fmt.Sprintf(`business_operations_success_total{operation="%s"}`, actualOperation), "5m")
	if err != nil {
		return nil, err
	}

	return &metrics, nil
}

// 获取消息处理指标
func (p *PrometheusClient) GetMessageProcessingMetrics(topic, operation string) (*MessageProcessingMetrics, error) {
	var metrics MessageProcessingMetrics
	var err error

	metrics.Time95, _ = p.Query(fmt.Sprintf(`histogram_quantile(0.95, sum(rate(kafka_message_processing_seconds_bucket{topic="%s",operation="%s"}[5m])) by (le))`, topic, operation))
	metrics.TimeAvg, _ = p.Query(fmt.Sprintf(`rate(kafka_message_processing_seconds_sum{topic="%s",operation="%s"}[5m]) / rate(kafka_message_processing_seconds_count{topic="%s",operation="%s"}[5m])`, topic, operation, topic, operation))
	metrics.Count, err = p.QueryRate(fmt.Sprintf(`kafka_message_processing_seconds_count{topic="%s",operation="%s"}`, topic, operation), "5m")
	if err != nil {
		return nil, err
	}

	// 错误率
	errorCount, _ := p.QueryRate(fmt.Sprintf(`kafka_consumer_processing_errors_total{topic="%s",operation="%s"}`, topic, operation), "5m")
	if metrics.Count > 0 {
		metrics.ErrorRate = (errorCount / metrics.Count) * 100
	}

	metrics.Throughput = metrics.Count

	return &metrics, nil
}

// 获取数据库指标
// 获取数据库指标（调试版本）
func (p *PrometheusClient) GetDatabaseMetrics() (*DatabaseMetrics, error) {
	var metrics DatabaseMetrics
	metrics.ErrorTypes = make(map[string]float64)
	metrics.Operations = make(map[string]float64)
	p.getErrorTypeDistribution(&metrics)

	// 获取操作类型分布
	p.getOperationDistribution(&metrics)
	// 调试输出
	log.Printf("正在查询数据库指标...")

	// 总查询次数
	queryCount, err := p.Query(`sum(rate(database_query_duration_seconds_count[5m]))`)
	if err != nil {
		log.Printf("查询次数查询失败: %v", err)
	} else {
		metrics.QueryCount = queryCount
		log.Printf("查询次数结果: %.2f", queryCount)
	}

	// 总错误数
	queryErrors, err := p.Query(`sum(rate(database_query_errors_total[5m]))`)
	if err != nil {
		log.Printf("错误数查询失败: %v", err)
	} else {
		metrics.QueryErrors = queryErrors
		log.Printf("错误数结果: %.2f", queryErrors)
	}

	// 查询时间分位数
	metrics.QueryTime95, _ = p.Query(`histogram_quantile(0.95, sum(rate(database_query_duration_seconds_bucket[5m])) by (le))`)
	log.Printf("P95查询时间: %.3f", metrics.QueryTime95)

	// 平均查询时间
	sum, _ := p.Query(`sum(rate(database_query_duration_seconds_sum[5m]))`)
	count, _ := p.Query(`sum(rate(database_query_duration_seconds_count[5m]))`)
	if count > 0 {
		metrics.QueryTimeAvg = sum / count
		log.Printf("平均查询时间: %.3f (sum=%.3f, count=%.3f)", metrics.QueryTimeAvg, sum, count)
	}

	// 错误率 - 修复计算逻辑
	if metrics.QueryCount > 0 {
		metrics.ErrorRate = (metrics.QueryErrors / metrics.QueryCount) * 100
	} else if metrics.QueryErrors > 0 {
		metrics.ErrorRate = 100 // 如果只有错误没有成功查询
	} else {
		metrics.ErrorRate = 0
	}
	log.Printf("错误率: %.1f%% (错误数=%.0f, 查询数=%.0f)", metrics.ErrorRate, metrics.QueryErrors, metrics.QueryCount)

	metrics.Throughput = metrics.QueryCount

	// 获取错误类型分布
	log.Printf("获取错误类型分布...")
	errorResults, err := p.queryWithLabels(`sum by (error_type) (rate(database_query_errors_total[5m]))`)
	if err != nil {
		log.Printf("错误类型查询失败: %v", err)
	} else {
		log.Printf("错误类型结果数量: %d", len(errorResults))
		for i, result := range errorResults {
			log.Printf("错误类型结果[%d]: %+v", i, result)
			if errorType, exists := result["error_type"]; exists {
				if value, exists := result["value"]; exists {
					if count, err := strconv.ParseFloat(value, 64); err == nil && count > 0 {
						metrics.ErrorTypes[errorType] = count
						log.Printf("错误类型 %s: %.0f", errorType, count)
					}
				}
			}
		}
	}

	// 获取操作类型分布
	log.Printf("获取操作类型分布...")
	operationResults, err := p.queryWithLabels(`sum by (operation) (rate(database_query_duration_seconds_count[5m]))`)
	if err != nil {
		log.Printf("操作类型查询失败: %v", err)
	} else {
		log.Printf("操作类型结果数量: %d", len(operationResults))
		for i, result := range operationResults {
			log.Printf("操作类型结果[%d]: %+v", i, result)
			if operation, exists := result["operation"]; exists {
				if value, exists := result["value"]; exists {
					if count, err := strconv.ParseFloat(value, 64); err == nil && count > 0 {
						metrics.Operations[operation] = count
						log.Printf("操作类型 %s: %.0f", operation, count)
					}
				}
			}
		}
	}

	log.Printf("数据库指标获取完成")
	return &metrics, nil
}

// 获取完整的实时指标
func (p *PrometheusClient) GetRealtimeMetrics(topic string, groups []string, operation string) (*Metrics, error) {
	var metrics Metrics
	var err error

	// 生产者指标
	producerMetrics, err := p.GetProducerMetrics(topic, operation)
	if err != nil {
		log.Printf("获取生产者指标失败: %v", err)
	} else {
		metrics.Producer = *producerMetrics
	}

	// 消费者组指标
	for _, group := range groups {
		groupMetrics, err := p.GetConsumerGroupMetrics(topic, group, operation)
		if err != nil {
			log.Printf("获取消费者组 %s 指标失败: %v", group, err)
			continue
		}
		metrics.ConsumerGroups = append(metrics.ConsumerGroups, *groupMetrics)
		metrics.TotalReceived += groupMetrics.Received
		metrics.TotalProcessed += groupMetrics.Processed
		metrics.TotalErrors += groupMetrics.Errors
		metrics.TotalRetry += groupMetrics.Retry
		metrics.TotalDeadLetter += groupMetrics.DeadLetter
	}

	// 业务指标
	businessMetrics, err := p.GetBusinessMetrics(operation)
	if err != nil {
		log.Printf("获取业务指标失败: %v", err)
	} else {
		metrics.Business = *businessMetrics
	}

	// 消息处理指标
	messageMetrics, err := p.GetMessageProcessingMetrics(topic, operation)
	if err != nil {
		log.Printf("获取消息处理指标失败: %v", err)
	} else {
		metrics.MessageProcessing = *messageMetrics
	}

	// 数据库指标
	databaseMetrics, err := p.GetDatabaseMetrics()
	if err != nil {
		log.Printf("获取数据库指标失败: %v", err)
	} else {
		metrics.Database = *databaseMetrics
	}

	return &metrics, nil
}

// 修正：数据库指标显示
// 完整的显示面板函数（修正版）
func displayDashboard(metrics *Metrics, config *Config) {
	fmt.Print("\033[2J\033[H")
	fmt.Printf("\033[1;34m=================================================\033[0m\n")
	fmt.Printf("\033[1;34m    Kafka全链路监控仪表板 (刷新间隔: %v)   \033[0m\n", config.Interval)
	fmt.Printf("\033[1;34m=================================================\033[0m\n")
	fmt.Printf("主题: %s  操作: %s  消费者组: %v\n", config.Topic, config.Operation, config.Groups)
	fmt.Printf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")

	// 1. 生产者指标
	fmt.Printf("\033[1;32m📊 生产者指标:\033[0m\n")
	fmt.Printf("   尝试: %-8.0f  成功: %-8.0f (%.1f%%)  失败: %-8.0f (%.1f%%)\n",
		metrics.Producer.Attempts, metrics.Producer.Success, metrics.Producer.SuccessRate,
		metrics.Producer.Failures, metrics.Producer.FailureRate)
	fmt.Printf("   重试: %-8.0f (%.1f%%)  吞吐量: %.1f msg/s\n",
		metrics.Producer.Retries, metrics.Producer.RetryRate, metrics.Producer.Throughput)

	// 2. 消费者组指标
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")
	fmt.Printf("\033[1;32m📊 消费者组指标 (总计: 接收=%.0f, 处理=%.0f, 错误=%.0f):\033[0m\n",
		metrics.TotalReceived, metrics.TotalProcessed, metrics.TotalErrors)

	for _, group := range metrics.ConsumerGroups {
		fmt.Printf("   %s:\n", group.Group)
		fmt.Printf("     接收: %-8.0f  处理: %-8.0f  错误: %-8.0f (%.1f%%)\n",
			group.Received, group.Processed, group.Errors, group.ErrorRate)
		fmt.Printf("     重试: %-8.0f (%.1f%%)  死信: %-8.0f (%.1f%%)  延迟: %.0f\n",
			group.Retry, group.RetryRate, group.DeadLetter, group.DeadLetterRate, group.Lag)
		fmt.Printf("     处理时间: P95=%.3fs, 平均=%.3fs  吞吐量: %.1f msg/s\n",
			group.ProcessingTime95, group.ProcessingTimeAvg, group.Throughput)
	}

	// 3. 消息处理指标
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")
	fmt.Printf("\033[1;32m📊 消息处理指标:\033[0m\n")
	fmt.Printf("   处理时间: P95=%-8.3fs  平均=%-8.3fs  吞吐量: %.1f msg/s\n",
		metrics.MessageProcessing.Time95, metrics.MessageProcessing.TimeAvg, metrics.MessageProcessing.Throughput)
	fmt.Printf("   错误率: %.1f%%\n", metrics.MessageProcessing.ErrorRate)

	// 4. 业务指标
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")
	fmt.Printf("\033[1;32m📊 业务指标:\033[0m\n")
	fmt.Printf("   成功: %-8.0f  失败: %-8.0f  成功率: %.1f%%\n",
		metrics.Business.Success, metrics.Business.Failures, metrics.Business.SuccessRate)
	fmt.Printf("   处理时间: P95=%-8.3fs  平均=%-8.3fs  吞吐量: %.1f ops/s\n",
		metrics.Business.ProcessingTime95, metrics.Business.ProcessingTimeAvg, metrics.Business.Throughput)

	// 5. 数据库指标
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")
	fmt.Printf("\033[1;32m📊 数据库指标:\033[0m\n")

	// 检查是否有有效数据
	hasDatabaseData := metrics.Database.QueryCount > 0 || metrics.Database.QueryErrors > 0
	hasErrorTypes := false
	hasOperations := false

	// 检查错误类型是否有有效数据
	for _, count := range metrics.Database.ErrorTypes {
		if count > 0.001 {
			hasErrorTypes = true
			break
		}
	}

	// 检查操作类型是否有有效数据
	for _, count := range metrics.Database.Operations {
		if count > 0.001 {
			hasOperations = true
			break
		}
	}

	if hasDatabaseData {
		fmt.Printf("   查询次数: %-8.3f/s  错误: %-8.3f (%.1f%%)  吞吐量: %.3f qps\n",
			metrics.Database.QueryCount, metrics.Database.QueryErrors,
			metrics.Database.ErrorRate, metrics.Database.Throughput)
		fmt.Printf("   查询时间: P95=%-8.3fs  平均=%-8.3fs\n",
			metrics.Database.QueryTime95, metrics.Database.QueryTimeAvg)
	} else {
		fmt.Printf("   📊 无数据库查询数据\n")
	}

	// 错误类型分布 - 只显示有数据的类型
	if hasErrorTypes {
		fmt.Printf("   🔴 错误类型分布:\n")
		totalErrors := metrics.Database.QueryErrors
		for errorType, count := range metrics.Database.ErrorTypes {
			if count > 0.001 {
				percentage := 0.0
				if totalErrors > 0 {
					percentage = (count / totalErrors) * 100
				}
				errorDesc := getErrorTypeDescription(errorType)
				fmt.Printf("     %-20s: %8.3f/s (%5.1f%%)\n", errorDesc, count, percentage)
			}
		}
	} else if hasDatabaseData {
		fmt.Printf("   🔴 错误类型: 无错误数据\n")
	}

	// 操作类型分布 - 只显示有数据的类型
	if hasOperations {
		fmt.Printf("   📋 操作类型分布:\n")
		totalOperations := metrics.Database.QueryCount
		for operation, count := range metrics.Database.Operations {
			if count > 0.001 {
				percentage := 0.0
				if totalOperations > 0 {
					percentage = (count / totalOperations) * 100
				}
				operationDesc := getOperationDescription(operation)
				fmt.Printf("     %-20s: %8.3f/s (%5.1f%%)\n", operationDesc, count, percentage)
			}
		}
	} else if hasDatabaseData {
		fmt.Printf("   📋 操作类型: 无操作数据\n")
	}

	// 6. 数据一致性检查
	fmt.Printf("\033[1;34m-------------------------------------------------\033[0m\n")
	fmt.Printf("\033[1;35m✅ 数据一致性检查:\033[0m\n")

	// 生产消费一致性
	producerConsumerDiff := metrics.Producer.Success - metrics.TotalReceived
	if math.Abs(producerConsumerDiff) > 100 {
		fmt.Printf("   🔴 生产消费不一致: 生产(%.0f) ≠ 消费(%.0f), 差异: %.0f\n",
			metrics.Producer.Success, metrics.TotalReceived, producerConsumerDiff)
	} else {
		fmt.Printf("   ✅ 生产消费一致: 生产=%.0f, 消费=%.0f\n",
			metrics.Producer.Success, metrics.TotalReceived)
	}

	// 错误处理一致性
	if metrics.TotalErrors > 0 {
		totalErrorHandling := metrics.TotalRetry + metrics.TotalDeadLetter
		errorHandlingDiff := metrics.TotalErrors - totalErrorHandling
		if math.Abs(errorHandlingDiff) > 10 {
			fmt.Printf("   🔴 错误处理不一致: 总错误(%.0f) ≠ 重试(%.0f)+死信(%.0f)=%.0f, 差异: %.0f\n",
				metrics.TotalErrors, metrics.TotalRetry, metrics.TotalDeadLetter, totalErrorHandling, errorHandlingDiff)
		} else {
			fmt.Printf("   ✅ 错误处理一致: 错误=%.0f, 重试+死信=%.0f\n",
				metrics.TotalErrors, totalErrorHandling)
		}
	} else {
		fmt.Printf("   ✅ 无错误需要处理\n")
	}

	// 业务处理一致性
	if metrics.TotalProcessed > 0 && metrics.Business.Success > 0 {
		businessDiff := metrics.TotalProcessed - metrics.Business.Success
		if math.Abs(businessDiff) > 50 {
			fmt.Printf("   🔴 业务处理不一致: 消费处理(%.0f) ≠ 业务成功(%.0f), 差异: %.0f\n",
				metrics.TotalProcessed, metrics.Business.Success, businessDiff)
		} else {
			fmt.Printf("   ✅ 业务处理一致: 消费处理=%.0f, 业务成功=%.0f\n",
				metrics.TotalProcessed, metrics.Business.Success)
		}
	} else {
		fmt.Printf("   ⚠️  业务数据不足: 处理=%.0f, 成功=%.0f\n",
			metrics.TotalProcessed, metrics.Business.Success)
	}

	// 数据库操作比例
	if metrics.TotalProcessed > 0 && metrics.Database.QueryCount > 0 {
		dbOperationRatio := metrics.Database.QueryCount / metrics.TotalProcessed
		if dbOperationRatio < 0.5 || dbOperationRatio > 5 {
			fmt.Printf("   ⚠️  数据库操作比例异常: 每个消息 %.1f 次数据库操作\n", dbOperationRatio)
		} else {
			fmt.Printf("   ✅ 数据库操作正常: 每个消息 %.1f 次数据库操作\n", dbOperationRatio)
		}
	}

	// 延迟检查
	highLagGroups := 0
	for _, group := range metrics.ConsumerGroups {
		if group.Lag > 1000 {
			highLagGroups++
			fmt.Printf("   🔴 高延迟: %s 组延迟 %.0f 条消息\n", group.Group, group.Lag)
		}
	}
	if highLagGroups == 0 {
		fmt.Printf("   ✅ 所有消费者组延迟正常\n")
	}

	// 错误率检查
	if metrics.Producer.FailureRate > 5 {
		fmt.Printf("   🔴 生产者错误率高: %.1f%%\n", metrics.Producer.FailureRate)
	} else {
		fmt.Printf("   ✅ 生产者错误率正常: %.1f%%\n", metrics.Producer.FailureRate)
	}

	fmt.Printf("\033[1;34m=================================================\033[0m\n")
}

func main() {
	// 默认配置
	config := &Config{
		PrometheusURL: "http://localhost:9090",
		Interval:      60 * time.Second,
		Topic:         "user.create.v1",
		Groups:        []string{"user-create-group", "user-service"},
		Operation:     "create",
		Table:         "users",
	}

	// 解析命令行参数
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-url":
			if i+1 < len(args) {
				config.PrometheusURL = args[i+1]
				i++
			}
		case "-interval":
			if i+1 < len(args) {
				if interval, err := strconv.Atoi(args[i+1]); err == nil {
					config.Interval = time.Duration(interval) * time.Second
				}
				i++
			}
		case "-topic":
			if i+1 < len(args) {
				config.Topic = args[i+1]
				i++
			}
		case "-groups":
			if i+1 < len(args) {
				config.Groups = strings.Split(args[i+1], ",")
				i++
			}
		case "-operation":
			if i+1 < len(args) {
				config.Operation = args[i+1]
				i++
			}
		case "-table":
			if i+1 < len(args) {
				config.Table = args[i+1]
				i++
			}
		case "-help":
			fmt.Println("Usage: kafka-full-monitor [options]")
			fmt.Println("Options:")
			fmt.Println("  -url <url>          Prometheus URL (default: http://localhost:9090)")
			fmt.Println("  -interval <seconds> Refresh interval in seconds (default: 5)")
			fmt.Println("  -topic <topic>      Kafka topic (default: user.create.v1)")
			fmt.Println("  -groups <groups>    Consumer groups (comma separated)")
			fmt.Println("  -operation <op>     Operation type (default: create)")
			fmt.Println("  -table <table>      Database table name (default: users)")
			fmt.Println("  -help               Show this help message")
			os.Exit(0)
		}
	}

	// 创建Prometheus客户端
	client, err := NewPrometheusClient(config.PrometheusURL)
	if err != nil {
		log.Fatalf("创建Prometheus客户端失败: %v", err)
	}

	fmt.Printf("\033[1;32m🚀 启动Kafka全链路监控...\033[0m\n")
	fmt.Printf("Prometheus地址: %s\n", config.PrometheusURL)
	fmt.Printf("监控配置: topic=%s, groups=%v, operation=%s, table=%s\n",
		config.Topic, config.Groups, config.Operation, config.Table)
	fmt.Printf("刷新间隔: %v\n", config.Interval)
	fmt.Printf("按 Ctrl+C 停止监控\n")
	time.Sleep(2 * time.Second)

	// 监控循环
	for {
		metrics, err := client.GetRealtimeMetrics(config.Topic, config.Groups, config.Operation)
		if err != nil {
			log.Printf("获取指标失败: %v", err)
			time.Sleep(config.Interval)
			continue
		}

		displayDashboard(metrics, config)
		time.Sleep(config.Interval)
	}
}

// 新增：查询带标签的结果
// 修正：查询带标签的结果（处理浮点数值）
func (p *PrometheusClient) queryWithLabels(query string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := p.client.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}

	if len(warnings) > 0 {
		log.Printf("Warnings: %v", warnings)
	}

	var results []map[string]string

	if result != nil && result.Type() == model.ValVector {
		vector := result.(model.Vector)
		for _, sample := range vector {
			resultMap := make(map[string]string)
			for key, value := range sample.Metric {
				resultMap[string(key)] = string(value)
			}
			// 修正：保留小数位，不要直接取整
			resultMap["value"] = fmt.Sprintf("%.6f", float64(sample.Value))
			results = append(results, resultMap)
		}
	}

	return results, nil
}

// 修正：获取错误类型和操作类型分布
// 获取错误类型分布
func (p *PrometheusClient) getErrorTypeDistribution(metrics *DatabaseMetrics) {
	errorResults, err := p.queryWithLabels(`sum by (error_type) (rate(database_query_errors_total[5m]))`)
	if err != nil {
		log.Printf("错误类型查询失败: %v", err)
		return
	}

	for _, result := range errorResults {
		if errorType, exists := result["error_type"]; exists {
			if valueStr, exists := result["value"]; exists {
				// 修正：正确解析浮点数值
				if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
					metrics.ErrorTypes[errorType] = value
					log.Printf("错误类型 %s: %f", errorType, value)
				} else {
					log.Printf("解析错误类型值失败: %s, error: %v", valueStr, err)
				}
			}
		}
	}
}

// 获取操作类型分布
func (p *PrometheusClient) getOperationDistribution(metrics *DatabaseMetrics) {
	operationResults, err := p.queryWithLabels(`sum by (operation) (rate(database_query_duration_seconds_count[5m]))`)
	if err != nil {
		log.Printf("操作类型查询失败: %v", err)
		return
	}

	for _, result := range operationResults {
		if operation, exists := result["operation"]; exists {
			if valueStr, exists := result["value"]; exists {
				// 修正：正确解析浮点数值
				if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
					metrics.Operations[operation] = value
					log.Printf("操作类型 %s: %f", operation, value)
				} else {
					log.Printf("解析操作类型值失败: %s, error: %v", valueStr, err)
				}
			}
		}
	}
}

// 错误类型映射
var errorTypeMapping = map[string]string{
	"constraint_violation":  "唯一索引冲突",
	"timeout":               "超时",
	"database_error":        "数据库错误",
	"network_error":         "网络错误",
	"deadlock":              "死锁",
	"foreign_key_violation": "外键约束冲突",
	"unique_violation":      "唯一约束冲突",
	"connection_error":      "连接错误",
	"permanent_error":       "永久错误",
	"unmarshal_error":       "数据解析错误",
}

// 操作类型映射
var operationMapping = map[string]string{
	"check_exists": "检查存在",
	"create":       "创建",
	"insert":       "插入",
	"select":       "查询",
	"update":       "更新",
	"delete":       "删除",
}

func getErrorTypeDescription(errorType string) string {
	if desc, exists := errorTypeMapping[errorType]; exists {
		return desc
	}
	return errorType
}

func getOperationDescription(operation string) string {
	if desc, exists := operationMapping[operation]; exists {
		return desc
	}
	return operation
}
