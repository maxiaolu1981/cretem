package apiserver

import (
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

type BloomFilter struct {
	filter *bloom.BloomFilter
	mu     sync.RWMutex
}

func NewBloomFilter(expectedElements uint, falsePositiveRate float64) *BloomFilter {
	return &BloomFilter{
		filter: bloom.NewWithEstimates(expectedElements, falsePositiveRate),
	}
}

// Add 添加字节数据到布隆过滤器
func (b *BloomFilter) Add(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.filter.Add(data)
}

// Test 检查字节数据是否可能存在于布隆过滤器中
func (b *BloomFilter) Test(data []byte) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filter.Test(data)
}

// AddString 添加字符串到布隆过滤器（字符串专用的便捷方法）
func (b *BloomFilter) AddString(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.filter.Add([]byte(s)) // 转换为 []byte
}

// TestString 检查字符串是否可能存在于布隆过滤器中（字符串专用的便捷方法）
func (b *BloomFilter) TestString(s string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filter.Test([]byte(s)) // 转换为 []byte
}

// Clear 清空布隆过滤器
func (b *BloomFilter) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.filter.ClearAll()
}
