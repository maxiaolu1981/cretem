package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	Group         string        `json:"group"`
	Operation     string        `json:"operation"`
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

// 查询带标签的结果
func (p *PrometheusClient) QueryWithLabels(query string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := p.client.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}

	if len(warnings) > 0 {
		log.Printf("Warnings: %v", warnings)
	}

	var results []string

	if result != nil && result.Type() == model.ValVector {
		vector := result.(model.Vector)
		for _, sample := range vector {
			results = append(results, fmt.Sprintf("%s: %.0f", sample.Metric, sample.Value))
		}
	}

	return results, nil
}

// 诊断函数：检查指标数据
func (p *PrometheusClient) DiagnoseMetrics(topic, group, operation string) {
	fmt.Printf("\n=== Kafka指标诊断 ===\n")
	fmt.Printf("诊断时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("主题: %s, 消费者组: %s, 操作: %s\n", topic, group, operation)
	fmt.Printf("========================================\n")

	// 1. 检查所有指标名称
	fmt.Printf("\n1. 检查所有指标名称:\n")
	metricNames := []string{
		"kafka_producer_success_total",
		"kafka_producer_failures_total",
		"kafka_consumer_messages_received_total",
		"kafka_consumer_messages_processed_total",
		"kafka_consumer_processing_errors_total",
		"kafka_consumer_retry_messages_total",
		"kafka_consumer_dead_letter_messages_total",
		"kafka_consumer_lag",
		"database_query_duration_seconds_count",
		"database_query_errors_total",
		"business_operations_success_total",
		"business_operations_failures_total",
	}

	for _, metric := range metricNames {
		query := fmt.Sprintf("count(%s)", metric)
		count, err := p.Query(query)
		if err != nil {
			fmt.Printf("  %s: 查询失败 - %v\n", metric, err)
		} else if count > 0 {
			fmt.Printf("  %s: ✅ 存在 (%.0f个时间序列)\n", metric, count)
		} else {
			fmt.Printf("  %s: ❌ 不存在\n", metric)
		}
	}

	// 2. 检查生产者指标
	fmt.Printf("\n2. 生产者指标详情:\n")
	producerQueries := map[string]string{
		"严格匹配": fmt.Sprintf(`kafka_producer_success_total{topic="%s",operation="%s"}`, topic, operation),
		"仅主题":  fmt.Sprintf(`kafka_producer_success_total{topic="%s"}`, topic),
		"全部":   `kafka_producer_success_total`,
	}

	for name, query := range producerQueries {
		result, err := p.Query(query)
		if err != nil {
			fmt.Printf("  %s: 查询失败 - %v\n", name, err)
		} else {
			fmt.Printf("  %s: %.0f (查询: %s)\n", name, result, query)
		}
	}

	// 3. 检查消费者指标
	fmt.Printf("\n3. 消费者指标详情:\n")
	consumerQueries := map[string]string{
		"严格匹配": fmt.Sprintf(`kafka_consumer_messages_received_total{topic="%s",group="%s",operation="%s"}`, topic, group, operation),
		"主题+组": fmt.Sprintf(`kafka_consumer_messages_received_total{topic="%s",group="%s"}`, topic, group),
		"仅主题":  fmt.Sprintf(`kafka_consumer_messages_received_total{topic="%s"}`, topic),
		"全部":   `kafka_consumer_messages_received_total`,
	}

	for name, query := range consumerQueries {
		result, err := p.Query(query)
		if err != nil {
			fmt.Printf("  %s: 查询失败 - %v\n", name, err)
		} else {
			fmt.Printf("  %s: %.0f (查询: %s)\n", name, result, query)
		}
	}

	// 4. 检查所有消费者组
	fmt.Printf("\n4. 所有消费者组的消费情况:\n")
	groupsResult, err := p.QueryWithLabels(`sum by (group) (kafka_consumer_messages_received_total)`)
	if err != nil {
		fmt.Printf("  查询失败: %v\n", err)
	} else {
		for _, result := range groupsResult {
			fmt.Printf("  %s\n", result)
		}
	}

	// 5. 检查数据库指标
	fmt.Printf("\n5. 数据库指标:\n")
	dbQueries := map[string]string{
		"总查询次数": `sum(database_query_duration_seconds_count)`,
		"总错误数":  `sum(database_query_errors_total)`,
		"5分钟速率": `sum(rate(database_query_duration_seconds_count[5m]))`,
	}

	for name, query := range dbQueries {
		result, err := p.Query(query)
		if err != nil {
			fmt.Printf("  %s: 查询失败 - %v\n", name, err)
		} else {
			fmt.Printf("  %s: %.0f\n", name, result)
		}
	}

	fmt.Printf("\n=== 诊断完成 ===\n")
}

func main() {
	// 默认配置
	config := &Config{
		PrometheusURL: "http://localhost:9090",
		Topic:         "user.create.v1",
		Group:         "user-service",
		Operation:     "create",
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
		case "-topic":
			if i+1 < len(args) {
				config.Topic = args[i+1]
				i++
			}
		case "-group":
			if i+1 < len(args) {
				config.Group = args[i+1]
				i++
			}
		case "-operation":
			if i+1 < len(args) {
				config.Operation = args[i+1]
				i++
			}
		case "-help":
			fmt.Println("Usage: kafka-diagnose [options]")
			fmt.Println("Options:")
			fmt.Println("  -url <url>          Prometheus URL (default: http://localhost:9090)")
			fmt.Println("  -topic <topic>      Kafka topic (default: user.create.v1)")
			fmt.Println("  -group <group>      Consumer group (default: user-service)")
			fmt.Println("  -operation <op>     Operation type (default: create)")
			fmt.Println("  -help               Show this help message")
			os.Exit(0)
		}
	}

	// 创建Prometheus客户端
	client, err := NewPrometheusClient(config.PrometheusURL)
	if err != nil {
		log.Fatalf("创建Prometheus客户端失败: %v", err)
	}

	fmt.Printf("开始Kafka指标诊断...\n")
	fmt.Printf("Prometheus地址: %s\n", config.PrometheusURL)

	// 运行诊断
	client.DiagnoseMetrics(config.Topic, config.Group, config.Operation)
}
