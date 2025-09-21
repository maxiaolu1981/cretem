package options

import (
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
	"github.com/spf13/pflag"
)

type BloomFilterOptions struct {
	Capacity          uint          `json:"capacity"            mapstructure:"capacity"            description:"布隆过滤器容量"`
	FalsePositiveRate float64       `json:"false_positive_rate" mapstructure:"false_positive_rate" description:"误判率"`
	AutoUpdate        bool          `json:"auto_update"         mapstructure:"auto_update"         description:"是否自动更新"`
	UpdateInterval    time.Duration `json:"update_interval"     mapstructure:"update_interval"     description:"自动更新间隔"`
	Enabled           bool          `json:"enabled"             mapstructure:"enabled"             description:"是否启用布隆过滤器"`
}

func NewBloomFilterOptions() *BloomFilterOptions {
	return &BloomFilterOptions{
		Capacity:          1000000,       // 默认100万容量
		FalsePositiveRate: 0.001,         // 默认0.1%误判率
		AutoUpdate:        true,          // 默认自动更新
		UpdateInterval:    1 * time.Hour, // 默认1小时更新一次
		Enabled:           true,          // 默认启用
	}
}

// Complete 完善配置，设置默认值（如果未设置）
func (o *BloomFilterOptions) Complete() {
	if o.Capacity == 0 {
		o.Capacity = 1000000
	}

	if o.FalsePositiveRate == 0 {
		o.FalsePositiveRate = 0.001
	}

	if o.UpdateInterval == 0 {
		o.UpdateInterval = 1 * time.Hour
	}
}

// Validate 验证BloomFilter配置项的合法性，返回错误列表
func (b *BloomFilterOptions) Validate() []error {
	errs := field.ErrorList{}
	path := field.NewPath("bloomFilter") // 基础路径，与配置结构层级对应

	// 验证Capacity必须大于0
	if b.Capacity == 0 {
		errs = append(errs, field.Invalid(
			path.Child("capacity"),
			b.Capacity,
			"capacity必须大于0",
		))
	}

	// 验证Capacity最大值限制
	if b.Capacity > 100000000 { // 1亿
		errs = append(errs, field.Invalid(
			path.Child("capacity"),
			b.Capacity,
			"capacity不能超过100,000,000",
		))
	}

	// 验证FalsePositiveRate必须在(0, 1)区间内
	if b.FalsePositiveRate <= 0 || b.FalsePositiveRate >= 1 {
		errs = append(errs, field.Invalid(
			path.Child("falsePositiveRate"), // 与结构体json标签保持一致
			b.FalsePositiveRate,
			"falsePositiveRate必须在(0, 1)之间",
		))
	}

	// 验证FalsePositiveRate最小值
	if b.FalsePositiveRate < 0.0001 { // 0.01%
		errs = append(errs, field.Invalid(
			path.Child("falsePositiveRate"),
			b.FalsePositiveRate,
			"falsePositiveRate不能小于0.0001，否则会导致内存使用过多",
		))
	}

	// 验证FalsePositiveRate最大值
	if b.FalsePositiveRate > 0.1 { // 10%
		errs = append(errs, field.Invalid(
			path.Child("falsePositiveRate"),
			b.FalsePositiveRate,
			"falsePositiveRate不能大于0.1，否则会导致误判率过高",
		))
	}

	// 验证UpdateInterval最小值
	if b.UpdateInterval > 0 && b.UpdateInterval < 1*time.Minute {
		errs = append(errs, field.Invalid(
			path.Child("updateInterval"),
			b.UpdateInterval,
			"updateInterval不能小于1分钟",
		))
	}

	// 验证UpdateInterval最大值
	if b.UpdateInterval > 24*time.Hour {
		errs = append(errs, field.Invalid(
			path.Child("updateInterval"),
			b.UpdateInterval,
			"updateInterval不能超过24小时",
		))
	}

	// 转换为聚合错误并返回
	agg := errs.ToAggregate()
	if agg == nil {
		return nil // 无错误时返回空切片
	}
	return agg.Errors()
}

// AddFlags 将BloomFilterOptions的字段添加为命令行标志
func (b *BloomFilterOptions) AddFlags(fs *pflag.FlagSet) {
	// 为Capacity字段添加命令行标志
	fs.UintVar(&b.Capacity, "bloom-filter.capacity", b.Capacity,
		"布隆过滤器的容量，即预计可存储的元素数量")

	// 为FalsePositiveRate字段添加命令行标志
	fs.Float64Var(&b.FalsePositiveRate, "bloom-filter.false-positive-rate", b.FalsePositiveRate,
		"布隆过滤器的误判率，取值范围通常在0到1之间")

	// 为AutoUpdate字段添加命令行标志
	fs.BoolVar(&b.AutoUpdate, "bloom-filter.auto-update", b.AutoUpdate,
		"是否启用布隆过滤器的自动更新功能")

	// 为UpdateInterval字段添加命令行标志
	fs.DurationVar(&b.UpdateInterval, "bloom-filter.update-interval", b.UpdateInterval,
		"布隆过滤器的自动更新间隔，例如：1h、30m")

	// 为Enabled字段添加命令行标志
	fs.BoolVar(&b.Enabled, "bloom-filter.enabled", b.Enabled,
		"是否启用布隆过滤器功能")
}
