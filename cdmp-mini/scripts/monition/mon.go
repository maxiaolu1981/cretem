package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	prometheusURL   = "http://localhost:9090"
	refreshInterval = 60 * time.Second // 延长刷新间隔
)

type MetricValue struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

type QueryResult struct {
	ResultType string        `json:"resultType"`
	Result     []MetricValue `json:"result"`
}

type PrometheusResponse struct {
	Status string      `json:"status"`
	Data   QueryResult `json:"data"`
}

type MonitorData struct {
	KafkaMetrics    map[string]float64
	RedisMetrics    map[string]float64
	CacheHitMetrics map[string]float64
	MySQLMetrics    map[string]float64
	SystemMetrics   map[string]float64 // 新增系统指标
	LastUpdated     time.Time
	mu              sync.RWMutex
}

// 简单的表格显示函数
func displayTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("  暂无数据")
		fmt.Println()
		return
	}

	// 计算每列的最大宽度
	colWidths := make([]int, len(headers))
	for i, header := range headers {
		colWidths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// 打印表头
	for i, header := range headers {
		fmt.Printf("%-*s", colWidths[i]+2, header)
	}
	fmt.Println()

	// 打印分隔线
	for i := 0; i < len(headers); i++ {
		fmt.Printf("%-*s", colWidths[i]+2, strings.Repeat("-", colWidths[i]))
	}
	fmt.Println()

	// 打印数据行
	for _, row := range rows {
		for i, cell := range row {
			fmt.Printf("%-*s", colWidths[i]+2, cell)
		}
		fmt.Println()
	}
	fmt.Println()
}

func main() {
	monitor := &MonitorData{
		KafkaMetrics:    make(map[string]float64),
		RedisMetrics:    make(map[string]float64),
		CacheHitMetrics: make(map[string]float64),
		MySQLMetrics:    make(map[string]float64),
		SystemMetrics:   make(map[string]float64),
	}

	// 启动数据收集
	go monitor.collectData()

	// 启动显示
	monitor.displayDashboard()
}

func (m *MonitorData) collectData() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.fetchAllMetrics()
	}
}

func (m *MonitorData) fetchAllMetrics() {
	var wg sync.WaitGroup

	// 只获取实际可用的指标
	wg.Add(3)
	go func() {
		defer wg.Done()
		m.fetchRedisMetrics()
	}()
	go func() {
		defer wg.Done()
		m.fetchMySQLMetrics()
	}()
	go func() {
		defer wg.Done()
		m.fetchSystemMetrics()
	}()

	wg.Wait()

	m.mu.Lock()
	m.LastUpdated = time.Now()
	m.mu.Unlock()
}

func (m *MonitorData) fetchRedisMetrics() {
	// 使用更通用的Redis指标查询
	queries := map[string]string{
		"redis_connected_clients":  `redis_connected_clients`,
		"redis_commands_processed": `rate(redis_commands_processed_total[1m])`,
		"redis_memory_used":        `redis_memory_used_bytes`,
		"redis_keyspace_hits":      `rate(redis_keyspace_hits_total[1m])`,
		"redis_keyspace_misses":    `rate(redis_keyspace_misses_total[1m])`,
	}

	results := make(map[string]float64)
	for name, query := range queries {
		if value, err := queryPrometheus(query); err == nil {
			results[name] = value
			fmt.Printf("✅ Redis指标 %s: %.2f\n", name, value)
		} else {
			fmt.Printf("❌ Redis指标 %s 查询失败: %v\n", name, err)
		}
	}

	m.mu.Lock()
	m.RedisMetrics = results
	m.mu.Unlock()
}

func (m *MonitorData) fetchMySQLMetrics() {
	// 使用更通用的MySQL指标查询
	queries := map[string]string{
		"mysql_connections":        `mysql_global_status_connections`,
		"mysql_queries_per_second": `rate(mysql_global_status_questions[1m])`,
		"mysql_slow_queries":       `rate(mysql_global_status_slow_queries[1m])`,
		"mysql_threads_connected":  `mysql_global_status_threads_connected`,
		"mysql_threads_running":    `mysql_global_status_threads_running`,
	}

	results := make(map[string]float64)
	for name, query := range queries {
		if value, err := queryPrometheus(query); err == nil {
			results[name] = value
			fmt.Printf("✅ MySQL指标 %s: %.2f\n", name, value)
		} else {
			fmt.Printf("❌ MySQL指标 %s 查询失败: %v\n", name, err)
		}
	}

	m.mu.Lock()
	m.MySQLMetrics = results
	m.mu.Unlock()
}

func (m *MonitorData) fetchSystemMetrics() {
	// 添加系统级别指标
	queries := map[string]string{
		"cpu_usage":         `100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[1m])) * 100)`,
		"memory_usage":      `(node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes * 100`,
		"disk_usage":        `(node_filesystem_size_bytes{mountpoint="/"} - node_filesystem_free_bytes{mountpoint="/"}) / node_filesystem_size_bytes{mountpoint="/"} * 100`,
		"node_up":           `up`,
		"processes_running": `node_procs_running`,
	}

	results := make(map[string]float64)
	for name, query := range queries {
		if value, err := queryPrometheus(query); err == nil {
			results[name] = value
			fmt.Printf("✅ 系统指标 %s: %.2f\n", name, value)
		} else {
			fmt.Printf("❌ 系统指标 %s 查询失败: %v\n", name, err)
		}
	}

	m.mu.Lock()
	m.SystemMetrics = results
	m.mu.Unlock()
}

func queryPrometheus(query string) (float64, error) {
	url := fmt.Sprintf("%s/api/v1/query?query=%s", prometheusURL, query)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取响应失败: %v", err)
	}

	var result PrometheusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("JSON解析失败: %v", err)
	}

	if result.Status != "success" {
		return 0, fmt.Errorf("Prometheus返回状态: %s", result.Status)
	}

	if len(result.Data.Result) == 0 {
		return 0, fmt.Errorf("查询无结果: %s", query)
	}

	// 解析数值
	valueStr, ok := result.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("数值格式错误")
	}

	var value float64
	_, err = fmt.Sscanf(valueStr, "%f", &value)
	if err != nil {
		return 0, fmt.Errorf("数值解析失败: %v", err)
	}

	return value, nil
}

func (m *MonitorData) displayDashboard() {
	for {
		time.Sleep(refreshInterval)
		m.clearScreen()
		m.displayHeader()
		m.displaySystemMetrics()
		m.displayRedisMetrics()
		m.displayMySQLMetrics()
		m.displayDataConsistencyRules()
	}
}

func (m *MonitorData) clearScreen() {
	fmt.Print("\033[2J")
	fmt.Print("\033[H")
}

func (m *MonitorData) displayHeader() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Printf("🚀 实时监控仪表板 - 最后更新: %s\n", m.LastUpdated.Format("2006-01-02 15:04:05"))
	fmt.Printf("📊 Prometheus地址: %s\n", prometheusURL)
	fmt.Println(strings.Repeat("=", 80))
}

func (m *MonitorData) displaySystemMetrics() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Println("\n🖥️  系统监控指标")
	fmt.Println(strings.Repeat("-", 40))

	headers := []string{"指标", "值", "状态"}
	var rows [][]string

	// 系统指标
	if cpu, exists := m.SystemMetrics["cpu_usage"]; exists {
		status := "🟢 正常"
		if cpu > 80 {
			status = "🔴 过高"
		} else if cpu > 60 {
			status = "🟡 警告"
		}
		rows = append(rows, []string{"CPU使用率", fmt.Sprintf("%.1f%%", cpu), status})
	}

	if memory, exists := m.SystemMetrics["memory_usage"]; exists {
		status := "🟢 正常"
		if memory > 85 {
			status = "🔴 过高"
		} else if memory > 70 {
			status = "🟡 警告"
		}
		rows = append(rows, []string{"内存使用率", fmt.Sprintf("%.1f%%", memory), status})
	}

	if disk, exists := m.SystemMetrics["disk_usage"]; exists {
		status := "🟢 正常"
		if disk > 90 {
			status = "🔴 过高"
		} else if disk > 80 {
			status = "🟡 警告"
		}
		rows = append(rows, []string{"磁盘使用率", fmt.Sprintf("%.1f%%", disk), status})
	}

	if up, exists := m.SystemMetrics["node_up"]; exists {
		status := "🟢 在线"
		if up == 0 {
			status = "🔴 离线"
		}
		rows = append(rows, []string{"节点状态", fmt.Sprintf("%.0f", up), status})
	}

	displayTable(headers, rows)
}

func (m *MonitorData) displayRedisMetrics() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Println("\n🔴 Redis 监控指标")
	fmt.Println(strings.Repeat("-", 40))

	headers := []string{"指标", "值", "状态"}
	var rows [][]string

	// 计算缓存命中率
	hits, hitsOk := m.RedisMetrics["redis_keyspace_hits"]
	misses, missesOk := m.RedisMetrics["redis_keyspace_misses"]
	if hitsOk && missesOk && (hits+misses) > 0 {
		hitRatio := hits / (hits + misses) * 100
		status := "🟢 优秀"
		if hitRatio < 80 {
			status = "🟡 一般"
		}
		if hitRatio < 60 {
			status = "🔴 较差"
		}
		rows = append(rows, []string{"命中率", fmt.Sprintf("%.1f%%", hitRatio), status})
	}

	// 显示可用的Redis指标
	for key, value := range m.RedisMetrics {
		status := m.getRedisStatus(key, value)
		displayName := strings.Replace(key, "redis_", "", 1)
		displayName = strings.ReplaceAll(displayName, "_", " ")

		var displayValue string
		if strings.Contains(key, "memory") {
			displayValue = fmt.Sprintf("%.1f MB", value/1024/1024)
		} else {
			displayValue = fmt.Sprintf("%.2f", value)
		}

		rows = append(rows, []string{displayName, displayValue, status})
	}

	// 按指标名称排序
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	displayTable(headers, rows)
}

func (m *MonitorData) getRedisStatus(metric string, value float64) string {
	switch metric {
	case "redis_connected_clients":
		if value > 1000 {
			return "🔴 过多"
		} else if value > 500 {
			return "🟡 警告"
		}
		return "🟢 正常"
	case "redis_memory_used_bytes":
		if value > 2*1024*1024*1024 { // 2GB
			return "🟡 较高"
		}
		return "🟢 正常"
	default:
		return "⚪ 正常"
	}
}

func (m *MonitorData) displayMySQLMetrics() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Println("\n🐬 MySQL 监控指标")
	fmt.Println(strings.Repeat("-", 40))

	headers := []string{"指标", "值", "状态"}
	var rows [][]string

	// 显示可用的MySQL指标
	for key, value := range m.MySQLMetrics {
		status := m.getMySQLStatus(key, value)
		displayName := strings.Replace(key, "mysql_", "", 1)
		displayName = strings.ReplaceAll(displayName, "_", " ")

		rows = append(rows, []string{displayName, fmt.Sprintf("%.2f", value), status})
	}

	// 按指标名称排序
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	displayTable(headers, rows)
}

func (m *MonitorData) getMySQLStatus(metric string, value float64) string {
	switch metric {
	case "mysql_slow_queries":
		if value > 10 {
			return "🔴 过多"
		} else if value > 5 {
			return "🟡 警告"
		}
		return "🟢 正常"
	case "mysql_connections":
		if value > 500 {
			return "🔴 过多"
		} else if value > 200 {
			return "🟡 警告"
		}
		return "🟢 正常"
	default:
		return "⚪ 正常"
	}
}

func (m *MonitorData) displayDataConsistencyRules() {
	fmt.Println("\n📋 数据健康检查")
	fmt.Println(strings.Repeat("-", 40))

	headers := []string{"检查项", "状态", "建议"}
	var rows [][]string

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 系统健康检查
	if up, exists := m.SystemMetrics["node_up"]; exists && up == 1 {
		rows = append(rows, []string{"节点状态", "🟢 健康", "系统运行正常"})
	} else {
		rows = append(rows, []string{"节点状态", "🔴 异常", "检查节点服务状态"})
	}

	// Redis健康检查
	if clients, exists := m.RedisMetrics["redis_connected_clients"]; exists && clients > 0 {
		rows = append(rows, []string{"Redis连接", "🟢 健康", "Redis服务正常"})
	} else {
		rows = append(rows, []string{"Redis连接", "🔴 异常", "检查Redis服务"})
	}

	// MySQL健康检查
	if connections, exists := m.MySQLMetrics["mysql_connections"]; exists && connections > 0 {
		rows = append(rows, []string{"MySQL连接", "🟢 健康", "MySQL服务正常"})
	} else {
		rows = append(rows, []string{"MySQL连接", "🔴 异常", "检查MySQL服务"})
	}

	// 内存检查
	if memory, exists := m.SystemMetrics["memory_usage"]; exists && memory > 90 {
		rows = append(rows, []string{"内存使用", "🔴 警告", "内存使用过高，建议清理"})
	} else if exists && memory > 70 {
		rows = append(rows, []string{"内存使用", "🟡 注意", "内存使用较高"})
	}

	displayTable(headers, rows)
}
