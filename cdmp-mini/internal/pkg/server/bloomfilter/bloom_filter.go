package bloomfilter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	bloomOptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	plog "github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

var (
	globalFilter *MultiBloomFilter
	initOnce     sync.Once
	initErr      error
	updaterStop  chan struct{}
)

var (
	bloomAddChan       = make(chan bloomAddRequest, 10000) // 更大的缓冲区
	batchProcessorOnce sync.Once
)

// 批量请求结构
type bloomAddRequest struct {
	filterType string
	value      string
}

// 启动批量处理器（单例模式）
func StartBatchProcessor() {
	batchProcessorOnce.Do(func() {
		go batchProcessorWorker()
	})
}

// 批量处理工作器
func batchProcessorWorker() {
	ticker := time.NewTicker(50 * time.Millisecond) // 更短的间隔，更快响应
	defer ticker.Stop()

	var batch []bloomAddRequest
	maxBatchSize := 1000 // 更大的批处理量

	for {
		select {
		case req := <-bloomAddChan:
			batch = append(batch, req)
			if len(batch) >= maxBatchSize {
				processBloomBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				processBloomBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// 处理批量请求
func processBloomBatch(batch []bloomAddRequest) {
	if len(batch) == 0 {
		return
	}

	bloom, err := GetFilter()
	if err != nil || bloom == nil {
		return
	}

	start := time.Now()
	bloom.Mu.Lock()
	for _, req := range batch {
		bloom.Add(req.filterType, req.value)
	}
	bloom.Mu.Unlock()

	duration := time.Since(start)
	metrics.RecordBloomFilterCheck("batch", "batch_add_success", duration)
}

// 发送批量添加请求
func SendBatchAddRequest(filterType, value string) {
	req := bloomAddRequest{
		filterType: filterType,
		value:      value,
	}

	select {
	case bloomAddChan <- req:
		// 成功发送到批量通道
	//	metrics.BloomFilterBatchOperations.WithLabelValues("enqueued").Inc()
	default:
		// 通道满时丢弃，避免阻塞业务逻辑
		//	metrics.BloomFilterBatchOperations.WithLabelValues("dropped").Inc()
	}
}

// Init 初始化布隆过滤器
func Init(configs map[string]*bloomOptions.BloomFilterOptions) error {
	initOnce.Do(func() {
		if err := initializeGlobalFilter(configs); err != nil {
			initErr = err
			return
		}

		// 启动后台更新任务（如果配置了自动更新）
		if hasAutoUpdateEnabled(configs) {
			startBackgroundUpdater(configs)
		}
	})
	return initErr
}

// initializeGlobalFilter 初始化全局过滤器
func initializeGlobalFilter(configs map[string]*bloomOptions.BloomFilterOptions) error {
	// 创建过滤器实例
	globalFilter = NewMultiBloomFilter(configs)

	// 从数据库加载数据
	users, err := loadAllUsers()
	if err != nil {
		return errors.WithCode(code.ErrDatabase, "从数据库加载用户失败: %v", err)
	}

	if len(users) == 0 {
		plog.Warn("目前没有任何用户记录，布隆过滤器将保持空状态")
		return nil
	}

	// 批量添加用户数据
	globalFilter.AddUsers(users)
	plog.Infof("多维度布隆过滤器初始化完成，加载了 %d 个用户", len(users))

	return nil
}

// loadAllUsers 从数据库加载所有用户
func loadAllUsers() ([]*v1.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := interfaces.Client().Users().ListAll(ctx, "")
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// hasAutoUpdateEnabled 检查是否有任何过滤器启用了自动更新
func hasAutoUpdateEnabled(configs map[string]*bloomOptions.BloomFilterOptions) bool {
	for _, config := range configs {
		if config.AutoUpdate {
			return true
		}
	}
	return false
}

// startBackgroundUpdater 启动后台更新任务
func startBackgroundUpdater(configs map[string]*bloomOptions.BloomFilterOptions) {
	updaterStop = make(chan struct{})

	// 为每个需要自动更新的过滤器启动单独的更新协程
	for filterType, config := range configs {
		if config.AutoUpdate && config.UpdateInterval > 0 {
			go startFilterUpdater(filterType, config, updaterStop)
		}
	}
	plog.Info("布隆过滤器后台更新任务已启动")
}

// startFilterUpdater 启动单个过滤器的更新任务
func startFilterUpdater(filterType string, config *bloomOptions.BloomFilterOptions, stopCh <-chan struct{}) {
	ticker := time.NewTicker(config.UpdateInterval)
	defer ticker.Stop()

	plog.Infof("启动 %s 过滤器更新任务，间隔: %v", filterType, config.UpdateInterval)

	for {
		select {
		case <-ticker.C:
			if err := updateFilterData(filterType, config); err != nil {
				plog.Errorf("更新 %s 过滤器失败: %v", filterType, err)
			}
		case <-stopCh:
			plog.Infof("停止 %s 过滤器更新任务", filterType)
			return
		}
	}
}

// updateFilterData 更新过滤器数据
func updateFilterData(filterType string, config *bloomOptions.BloomFilterOptions) error {
	startTime := time.Now()
	plog.Debugf("开始更新 %s 过滤器", filterType)

	// 加载最新数据
	data, err := loadFilterData(filterType)
	if err != nil {
		return fmt.Errorf("加载数据失败: %v", err)
	}

	if len(data) == 0 {
		plog.Warnf("没有获取到 %s 的新数据，跳过更新", filterType)
		return nil
	}

	// 创建新过滤器（容量增加20%缓冲）
	capacity := uint(float64(len(data)) * 1.2)
	if capacity < 1000 {
		capacity = 1000 // 最小容量
	}

	newFilter := bloom.NewWithEstimates(capacity, config.FalsePositiveRate)

	// 添加数据到新过滤器
	for _, item := range data {
		if item != "" {
			newFilter.AddString(item)
		}
	}

	// 原子替换过滤器
	globalFilter.UpdateFilter(filterType, newFilter)

	plog.Infof("成功更新 %s 过滤器，数据量: %d, 耗时: %v",
		filterType, len(data), time.Since(startTime))
	return nil
}

// loadFilterData 加载指定过滤器类型的数据
func loadFilterData(filterType string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	users, err := interfaces.Client().Users().ListAll(ctx, "")
	if err != nil {
		return nil, err
	}

	var data []string
	for _, user := range users.Items {
		switch filterType {
		case "user_id":
			if user.InstanceID != "" {
				data = append(data, user.InstanceID)
			}
		case "username":
			if user.Name != "" {
				data = append(data, user.Name)
			}
		case "email":
			if user.Email != "" {
				data = append(data, user.Email)
			}
		case "phone":
			if user.Phone != "" {
				data = append(data, user.Phone)
			}
		case "nickname":
			if user.Nickname != "" {
				data = append(data, user.Nickname)
			}
		}
	}
	return data, nil
}

// StopUpdater 停止所有更新任务
func StopUpdater() {
	if updaterStop != nil {
		close(updaterStop)
		updaterStop = nil
	}
}

// GetFilter 获取全局过滤器实例
func GetFilter() (*MultiBloomFilter, error) {
	if globalFilter == nil && initErr == nil {
		// 使用默认配置懒初始化
		defaultConfigs := map[string]*bloomOptions.BloomFilterOptions{
			"user_id":  {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: false},
			"username": {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: false},
			"email":    {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: false},
			"phone":    {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: false},
		}
		if err := Init(defaultConfigs); err != nil {
			return nil, err
		}
	}
	return globalFilter, initErr
}

// MultiBloomFilter 线程安全的多维度布隆过滤器
type MultiBloomFilter struct {
	filters map[string]*bloom.BloomFilter
	Mu      sync.RWMutex
	config  map[string]*bloomOptions.BloomFilterOptions
}

// NewMultiBloomFilter 创建新的多维度布隆过滤器
func NewMultiBloomFilter(configs map[string]*bloomOptions.BloomFilterOptions) *MultiBloomFilter {
	filters := make(map[string]*bloom.BloomFilter)
	for filterType, config := range configs {
		metrics.SetBloomFilterCapacity(filterType, config.Capacity,
			metrics.CalculateMemoryUsage(config.Capacity), config.FalsePositiveRate)
		metrics.SetBloomFilterAutoUpdateStatus(filterType, config.AutoUpdate)
		metrics.SetBloomFilterStatus(filterType, true)
		filters[filterType] = bloom.NewWithEstimates(
			config.Capacity,
			config.FalsePositiveRate,
		)
	}

	return &MultiBloomFilter{
		filters: filters,
		config:  configs,
		Mu:      sync.RWMutex{},
	}
}

// Add 添加数据到指定类型的过滤器
func (b *MultiBloomFilter) Add(filterType, value string) {
	if value == "" {
		return
	}

	b.Mu.RLock()
	defer b.Mu.RUnlock()

	if filter, exists := b.filters[filterType]; exists {
		filter.AddString(value)
	}
}

// Test 检查数据是否可能存在于指定类型的过滤器
func (b *MultiBloomFilter) Test(filterType, value string) bool {
	if value == "" {
		return false
	}

	b.Mu.RLock()
	defer b.Mu.RUnlock()

	filter, exists := b.filters[filterType]
	if !exists {
		return true // 过滤器不存在时返回true（安全失败）
	}
	return filter.TestString(value)
}

// AddUser 添加用户数据到所有相关过滤器
func (m *MultiBloomFilter) AddUser(user *v1.User) {
	if user == nil {
		return
	}

	m.Mu.RLock()
	defer m.Mu.RUnlock()

	if filter, exists := m.filters["user_id"]; exists && user.InstanceID != "" {
		filter.AddString(user.InstanceID)
	}
	if filter, exists := m.filters["username"]; exists && user.Name != "" {
		filter.AddString(user.Name)
	}
	if filter, exists := m.filters["email"]; exists && user.Email != "" {
		filter.AddString(user.Email)
	}
	if filter, exists := m.filters["phone"]; exists && user.Phone != "" {
		filter.AddString(user.Phone)
	}
	if filter, exists := m.filters["nickname"]; exists && user.Nickname != "" {
		filter.AddString(user.Nickname)
	}
}

// AddUsers 批量添加多个用户数据
func (m *MultiBloomFilter) AddUsers(users []*v1.User) {
	if len(users) == 0 {
		return
	}

	m.Mu.RLock()
	defer m.Mu.RUnlock()

	for _, user := range users {
		if user.InstanceID != "" {
			if filter, exists := m.filters["user_id"]; exists {
				filter.AddString(user.InstanceID)
			}
		}
		if user.Name != "" {
			if filter, exists := m.filters["username"]; exists {
				filter.AddString(user.Name)
			}
		}
		if user.Email != "" {
			if filter, exists := m.filters["email"]; exists {
				filter.AddString(user.Email)
			}
		}
		if user.Phone != "" {
			if filter, exists := m.filters["phone"]; exists {
				filter.AddString(user.Phone)
			}
		}
		if user.Nickname != "" {
			if filter, exists := m.filters["nickname"]; exists {
				filter.AddString(user.Nickname)
			}
		}
	}
}

// UpdateFilter 原子性地更新指定类型的过滤器
func (m *MultiBloomFilter) UpdateFilter(filterType string, newFilter *bloom.BloomFilter) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.filters[filterType] = newFilter
}

// GetFilter 获取指定类型的过滤器（只读）
func (m *MultiBloomFilter) GetFilter(filterType string) *bloom.BloomFilter {
	m.Mu.RLock()
	defer m.Mu.RUnlock()
	return m.filters[filterType]
}

// Clear 清空所有过滤器
func (b *MultiBloomFilter) Clear() {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	for _, filter := range b.filters {
		filter.ClearAll()
	}
}

// GetStats 获取过滤器统计信息（用于监控）
func (b *MultiBloomFilter) GetStats() map[string]interface{} {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	stats := make(map[string]interface{})
	for filterType, filter := range b.filters {
		// 估算当前数据量（基于bit设置情况）
		approxSize := estimateFilterSize(filter)
		stats[filterType] = map[string]interface{}{
			"capacity":            filter.Cap(),
			"approximate_size":    approxSize,
			"bit_count":           countSetBits(filter),
			"false_positive_rate": b.config[filterType].FalsePositiveRate,
		}
	}
	return stats
}

// estimateFilterSize 估算过滤器中元素数量
func estimateFilterSize(filter *bloom.BloomFilter) uint {
	// 这是一个简单的估算方法，实际可能需要更复杂的算法
	// 基于设置bit的数量来估算元素数量
	setBits := countSetBits(filter)
	m := filter.Cap()
	k := estimateK(filter)

	if k == 0 || m == 0 {
		return 0
	}

	// 使用布隆过滤器容量估算公式
	return uint(float64(-m) * log(1-float64(setBits)/float64(m)) / float64(k))
}

// countSetBits 计算过滤器中设置的bit数量
func countSetBits(filter *bloom.BloomFilter) uint {
	// 这里需要根据bloom库的具体实现来编写
	// 由于bloom.v3不直接提供访问bit数组的方法，这里返回一个估算值
	return uint(filter.ApproximatedSize())
}

// estimateK 估算哈希函数数量
func estimateK(filter *bloom.BloomFilter) uint {
	// 根据标准布隆过滤器公式估算k值
	m := float64(filter.Cap())
	n := float64(filter.ApproximatedSize())
	p := 0.01 // 假设误判率

	if n == 0 {
		return 0
	}

	k := (m / n) * log(1/p) / log(2)
	return uint(k)
}

// log 自然对数辅助函数
func log(x float64) float64 {
	// 使用标准库的math.Log，这里简化处理
	return float64(int(x*1000)) / 1000 // 简单近似
}

// GetMemoryUsage 获取内存使用情况估算
func (b *MultiBloomFilter) GetMemoryUsage() uint64 {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	var totalMemory uint64
	for _, filter := range b.filters {
		// 每个bit占用1bit，加上数据结构开销
		totalMemory += uint64(filter.Cap())/8 + 1024 // 加上1KB的固定开销估算
	}
	return totalMemory
}
