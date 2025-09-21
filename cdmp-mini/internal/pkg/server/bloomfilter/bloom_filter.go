package bloomfilter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	bloomOptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

var (
	globalFilter *MultiBloomFilter
	initOnce     sync.Once
	initErr      error
	globalMu     sync.RWMutex         // 保护全局变量
	updaterStop  chan struct{}        // 用于停止更新任务
	lastUpdate   map[string]time.Time // 记录各过滤器的最后更新时间
	lastUpdateMu sync.RWMutex         // 保护lastUpdate
)

func Init(configs map[string]*bloomOptions.BloomFilterOptions) error {
	initOnce.Do(func() {
		globalMu.Lock()
		defer globalMu.Unlock()

		globalFilter = NewBloomFilter(configs)

		// 从数据库加载所有用户名
		users, err := interfaces.Client().Users().ListAll(context.TODO(), "")
		if err != nil {
			initErr = errors.WithCode(code.ErrDatabase, "从数据库加载用户失败:加载boom失效 %v", err)
			return
		}
		if len(users.Items) == 0 {
			log.Warn("目前没有任何用户记录,未初始化布隆过滤器")
			return
		}

		// 添加所有用户标识符到布隆过滤器
		for _, user := range users.Items {
			globalFilter.Add("user_id", user.InstanceID)
			globalFilter.Add("username", user.Name)
			globalFilter.Add("email", user.Email)
			globalFilter.Add("phone", user.Phone)
		}

		log.Debugf("多维度布隆过滤器初始化完成，加载了 %d 个用户", len(users.Items))

		// 初始化最后更新时间
		lastUpdate = make(map[string]time.Time)
		for filterType := range configs {
			lastUpdate[filterType] = time.Now()
		}

		// 启动定期更新任务
		if configs["user_id"].AutoUpdate {
			updaterStop = make(chan struct{})
			go startBloomFilterUpdater(configs, updaterStop)
		}
	})
	return initErr
}

// startBloomFilterUpdater 启动布隆过滤器更新任务
func startBloomFilterUpdater(configs map[string]*options.BloomFilterOptions, stopCh <-chan struct{}) {
	// 获取检查频率（取最短更新间隔的一半或最小1分钟）
	checkInterval := getShortestUpdateInterval(configs) / 2
	if checkInterval < time.Minute {
		checkInterval = time.Minute
	}

	log.Infof("布隆过滤器更新任务启动，检查间隔: %v", checkInterval)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 检查每个过滤器是否需要更新
			for filterType, config := range configs {
				if config.AutoUpdate && config.UpdateInterval > 0 {
					if shouldUpdate(filterType, config.UpdateInterval) {
						if err := updateSingleFilter(filterType, config); err != nil {
							log.Errorf("更新布隆过滤器 %s 失败: %v", filterType, err)
						}
					}
				}
			}
		case <-stopCh:
			log.Info("布隆过滤器更新任务停止")
			return
		}
	}
}

// getShortestUpdateInterval 获取配置中最短的更新间隔
func getShortestUpdateInterval(configs map[string]*options.BloomFilterOptions) time.Duration {
	var shortestInterval time.Duration

	for _, config := range configs {
		if config.AutoUpdate && config.UpdateInterval > 0 {
			if shortestInterval == 0 || config.UpdateInterval < shortestInterval {
				shortestInterval = config.UpdateInterval
			}
		}
	}

	// 设置合理的默认值和下限
	if shortestInterval == 0 {
		shortestInterval = 1 * time.Hour // 默认1小时
	}

	// 防止间隔太短，影响性能
	if shortestInterval < 1*time.Minute {
		shortestInterval = 1 * time.Minute
	}

	return shortestInterval
}

// shouldUpdate 检查指定过滤器是否需要更新
func shouldUpdate(filterType string, updateInterval time.Duration) bool {
	lastUpdateMu.RLock()
	defer lastUpdateMu.RUnlock()

	lastUpdateTime, exists := lastUpdate[filterType]
	if !exists {
		return true // 从未更新过，需要更新
	}

	return time.Since(lastUpdateTime) >= updateInterval
}

// updateSingleFilter 更新单个过滤器
func updateSingleFilter(filterType string, config *options.BloomFilterOptions) error {
	log.Infof("开始更新布隆过滤器: %s", filterType)

	// 获取最新数据
	newData, err := loadLatestData(filterType)
	if err != nil {
		return fmt.Errorf("加载数据失败: %v", err)
	}

	if len(newData) == 0 {
		log.Warnf("没有获取到 %s 的新数据，跳过更新", filterType)
		return nil
	}

	// 创建新的布隆过滤器
	newFilter := bloom.NewWithEstimates(
		uint(float64(len(newData))*1.5), // 预留50%的空间
		config.FalsePositiveRate,
	)

	// 添加数据到新过滤器
	for _, item := range newData {
		if item != "" {
			newFilter.AddString(item)
		}
	}

	// 原子替换过滤器
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalFilter == nil {
		return errors.New("布隆过滤器未初始化")
	}

	// 使用MultiBloomFilter的更新方法
	globalFilter.UpdateFilter(filterType, newFilter)

	// 更新最后更新时间
	lastUpdateMu.Lock()
	lastUpdate[filterType] = time.Now()
	lastUpdateMu.Unlock()

	log.Infof("布隆过滤器 %s 更新完成，数据量: %d", filterType, len(newData))
	return nil
}

// loadLatestData 加载指定类型的最新数据
func loadLatestData(filterType string) ([]string, error) {
	// 从数据库加载数据
	users, err := interfaces.Client().Users().ListAll(context.TODO(), "")
	if err != nil {
		return nil, err
	}

	var results []string
	for _, user := range users.Items {
		switch filterType {
		case "user_id":
			if user.InstanceID != "" {
				results = append(results, user.InstanceID)
			}
		case "username":
			if user.Name != "" {
				results = append(results, user.Name)
			}
		case "email":
			if user.Email != "" {
				results = append(results, user.Email)
			}
		case "phone":
			if user.Phone != "" {
				results = append(results, user.Phone)
			}
		}
	}

	return results, nil
}

// StopUpdater 停止更新任务
func StopUpdater() {
	if updaterStop != nil {
		close(updaterStop)
	}
}

func GetFilter() (*MultiBloomFilter, error) {
	if globalFilter == nil && initErr == nil {
		// 使用默认配置懒初始化
		defaultConfigs := map[string]*bloomOptions.BloomFilterOptions{
			"user_id":  {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: true, UpdateInterval: 1 * time.Hour},
			"username": {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: true, UpdateInterval: 1 * time.Hour},
			"email":    {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: true, UpdateInterval: 1 * time.Hour},
			"phone":    {Capacity: 1000000, FalsePositiveRate: 0.001, AutoUpdate: true, UpdateInterval: 1 * time.Hour},
		}
		if err := Init(defaultConfigs); err != nil {
			return nil, err
		}
	}
	return globalFilter, initErr
}

type MultiBloomFilter struct {
	filters map[string]*bloom.BloomFilter
	Mu      sync.RWMutex
	config  map[string]*options.BloomFilterOptions
}

func NewBloomFilter(configs map[string]*options.BloomFilterOptions) *MultiBloomFilter {
	m := &MultiBloomFilter{
		filters: make(map[string]*bloom.BloomFilter),
		config:  configs,
		Mu:      sync.RWMutex{},
	}
	for filterType, config := range configs {
		m.filters[filterType] = bloom.NewWithEstimates(
			config.Capacity,
			config.FalsePositiveRate,
		)
	}
	return m
}

// Add 添加字节数据到布隆过滤器
func (b *MultiBloomFilter) Add(filterType, value string) {
	if value == "" {
		return
	}
	b.Mu.RLock()
	filter, ok := b.filters[filterType]
	b.Mu.RUnlock()

	if ok {
		filter.AddString(value)
	}
}

// Test 检查字节数据是否可能存在于布隆过滤器中
func (b *MultiBloomFilter) Test(filterType, value string) bool {
	b.Mu.RLock()
	filter, ok := b.filters[filterType]
	b.Mu.RUnlock()
	if !ok {
		return true
	}
	return filter.TestString(value)
}

func (m *MultiBloomFilter) AddUser(user *v1.User) {
	m.Add("user_id", user.InstanceID)
	m.Add("username", user.Name)
	m.Add("email", user.Email)
	m.Add("phone", user.Phone)
	m.Add("nickname", user.Nickname)
}

// 获取指定类型的过滤器
func (m *MultiBloomFilter) GetFilter(filterType string) *bloom.BloomFilter {
	m.Mu.RLock()
	defer m.Mu.RUnlock()
	return m.filters[filterType]
}

// UpdateFilter 更新指定类型的过滤器（线程安全）
func (m *MultiBloomFilter) UpdateFilter(filterType string, newFilter *bloom.BloomFilter) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.filters[filterType] = newFilter
}

// Clear 清空布隆过滤器
func (b *MultiBloomFilter) Clear() {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	for _, filter := range b.filters {
		filter.ClearAll()
	}
}
