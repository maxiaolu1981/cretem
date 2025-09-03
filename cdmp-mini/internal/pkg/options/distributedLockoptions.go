package options

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

// DistributedLockOptions 分布式锁全局配置入口
// 作用：统一管理分布式锁的全局开关、默认行为，以及各业务的差异化配置
type DistributedLockOptions struct {
	// Enabled 分布式锁全局总开关
	// true=启用，false=全局禁用（开发环境常用）
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// DefaultTimeout 全局默认锁超时时间
	// 未单独配置的业务将使用此值，单位支持s/ms（如5s）
	// 作用：防止进程崩溃导致死锁，超时后自动释放
	DefaultTimeout time.Duration `json:"default-timeout" mapstructure:"default-timeout"`

	// DefaultRetry 全局默认锁操作重试配置
	// 控制获取锁失败时的重试策略（次数、间隔等）
	DefaultRetry *RetryOptions `json:"default-retry" mapstructure:"default-retry"`

	// Redis 分布式锁专用Redis配置
	// 补充全局Redis配置中与锁相关的特殊参数（如键前缀、数据库索引）
	Redis *RedisLockOptions `json:"redis" mapstructure:"redis"`

	// Business 业务级细粒度锁配置
	// 按业务名称配置差异化策略（如支付业务强制加锁）
	Business map[string]*BusinessLockOptions `json:"business" mapstructure:"business"`

	// Monitoring 锁操作监控与告警配置
	// 监控锁的性能和成功率，异常时触发告警
	Monitoring *MonitoringOptions `json:"monitoring" mapstructure:"monitoring"`

	// Fallback 锁容错降级配置
	// Redis不可用时的兜底策略（失败/跳过锁）
	Fallback *FallbackOptions `json:"fallback" mapstructure:"fallback"`
}

// RetryOptions 锁操作重试配置
type RetryOptions struct {
	// MaxCount 最大重试次数
	// 0=不重试，3=最多重试3次（共4次尝试）
	MaxCount int `json:"max-count" mapstructure:"max-count"`

	// Interval 重试间隔时间
	// 如100ms，配合BackoffType决定实际间隔
	Interval time.Duration `json:"interval" mapstructure:"interval"`

	// BackoffType 重试策略
	// "fixed"=固定间隔，"exponential"=指数退避
	BackoffType string `json:"backoff-type" mapstructure:"backoff-type"`
}

// RedisLockOptions 分布式锁专用Redis配置
type RedisLockOptions struct {
	// KeyPrefix 锁键统一前缀
	// 避免与其他Redis数据冲突（如"distributed:lock:"）
	KeyPrefix string `json:"key-prefix" mapstructure:"key-prefix"`

	// DB Redis数据库索引
	// 锁数据存储的数据库（0-15），与其他业务隔离
	DB int `json:"db" mapstructure:"db"`

	// KeepAlive 是否启用锁续约
	// true=自动延长锁有效期（解决业务耗时超超时问题）
	KeepAlive bool `json:"keep-alive" mapstructure:"keep-alive"`

	// KeepAliveInterval 锁续约间隔
	// 建议为超时时间的1/3（如5s超时则1.6s续约一次）
	KeepAliveInterval time.Duration `json:"keep-alive-interval" mapstructure:"keep-alive-interval"`
}

// BusinessLockOptions 业务级锁配置
type BusinessLockOptions struct {
	// Enabled 该业务是否启用锁
	// 优先级高于全局开关（true=启用，false=禁用）
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// Timeout 该业务的锁超时时间
	// 覆盖全局DefaultTimeout（如支付业务设10s）
	Timeout time.Duration `json:"timeout" mapstructure:"timeout"`

	// Retry 该业务的重试配置
	// 覆盖全局DefaultRetry，为nil则使用全局配置
	Retry *RetryOptions `json:"retry" mapstructure:"retry"`

	// ForceLock 是否强制加锁（无视全局开关）
	// true=核心业务强制加锁（如支付），false=遵循全局配置
	ForceLock bool `json:"force-lock" mapstructure:"force-lock"`
}

// MonitoringOptions 监控告警配置
type MonitoringOptions struct {
	// Enabled 是否启用监控
	// true=记录锁操作指标并告警，false=禁用
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// SlowThreshold 慢锁阈值
	// 超过此时长则记录告警（如1s）
	SlowThreshold time.Duration `json:"slow-threshold" mapstructure:"slow-threshold"`

	// FailThreshold 失败次数阈值
	// 单位时间内失败超此时数则告警（如5次）
	FailThreshold int `json:"fail-threshold" mapstructure:"fail-threshold"`
}

// FallbackOptions 容错降级配置
type FallbackOptions struct {
	// Enabled 是否启用降级
	// true=Redis不可用时触发降级，false=不启用
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// RedisDownAction Redis不可用时的动作
	// "fail"=返回失败，"skip"=跳过锁继续执行
	RedisDownAction string `json:"redis-down-action" mapstructure:"redis-down-action"`

	// MaxQueueSize 降级时请求队列大小
	// 防止请求突增压垮系统（如1000）
	MaxQueueSize int `json:"max-queue-size" mapstructure:"max-queue-size"`
}

// NewDistributedLockOptions 创建分布式锁配置的默认实例
// 作用：初始化所有配置项的默认值，避免空指针或不合理默认
func NewDistributedLockOptions() *DistributedLockOptions {
	return &DistributedLockOptions{
		Enabled:        true,            // 与配置一致：默认启用
		DefaultTimeout: 5 * time.Second, // 全局默认超时5s（匹配配置）
		DefaultRetry: &RetryOptions{
			MaxCount:    3,                      // 全局默认重试3次（匹配配置）
			Interval:    100 * time.Millisecond, // 全局默认间隔100ms（匹配配置）
			BackoffType: "fixed",                // 全局默认固定间隔（匹配配置）
		},
		Redis: &RedisLockOptions{
			KeyPrefix:         "distributed:lock:", // 锁键前缀（匹配配置）
			DB:                1,                   // 修正：锁数据存储在DB=1（匹配配置）
			KeepAlive:         true,                // 启用锁续约（匹配配置）
			KeepAliveInterval: 2 * time.Second,     // 续约间隔2s（匹配配置）
		},
		Business: map[string]*BusinessLockOptions{
			// 初始化user:delete业务的细粒度配置（匹配配置）
			"user:delete": {
				ForceLock: true,             // 强制加锁（核心业务）
				Timeout:   10 * time.Second, // 业务超时10s（覆盖全局）
				Retry: &RetryOptions{
					MaxCount:    5,                      // 业务重试5次（覆盖全局）
					Interval:    200 * time.Millisecond, // 业务间隔200ms（覆盖全局）
					BackoffType: "fixed",                // 固定间隔（继承全局默认）
				},
			},
		},
		Monitoring: &MonitoringOptions{
			Enabled:       true,            // 启用监控（匹配配置）
			SlowThreshold: 1 * time.Second, // 慢锁阈值1s（匹配配置）
			FailThreshold: 5,               // 失败5次告警（匹配配置）
		},
		Fallback: &FallbackOptions{
			Enabled:         true,   // 启用降级（匹配配置）
			RedisDownAction: "fail", // Redis不可用时返回失败（匹配配置）
			MaxQueueSize:    1000,   // 降级队列大小1000（匹配配置）
		},
	}
}

// Complete 补全配置选项（处理依赖关系和默认值）
// 作用：确保所有配置项都有合理值，即使配置文件中未显式设置
func (d *DistributedLockOptions) Complete() {
	// 补全默认重试配置（防止nil或无效值）
	if d.DefaultRetry == nil {
		d.DefaultRetry = &RetryOptions{}
	}
	if d.DefaultRetry.MaxCount <= 0 {
		d.DefaultRetry.MaxCount = 3 // 默认重试3次
	}
	if d.DefaultRetry.Interval <= 0 {
		d.DefaultRetry.Interval = 100 * time.Millisecond // 默认间隔100ms
	}
	if d.DefaultRetry.BackoffType == "" {
		d.DefaultRetry.BackoffType = "fixed" // 默认固定间隔
	}

	// 补全Redis锁配置
	if d.Redis == nil {
		d.Redis = &RedisLockOptions{}
	}
	if d.Redis.KeyPrefix == "" {
		d.Redis.KeyPrefix = "distributed:lock:" // 默认键前缀
	}
	if d.Redis.DB < 0 {
		d.Redis.DB = 0 // 数据库索引不能为负，默认0
	}
	// 自动计算续约间隔（如果未配置且启用了续约）
	if d.Redis.KeepAlive && d.Redis.KeepAliveInterval <= 0 {
		d.Redis.KeepAliveInterval = d.DefaultTimeout / 3
	}

	// 补全监控配置
	if d.Monitoring == nil {
		d.Monitoring = &MonitoringOptions{}
	}
	if d.Monitoring.SlowThreshold <= 0 {
		d.Monitoring.SlowThreshold = 1 * time.Second // 默认慢锁阈值1s
	}
	if d.Monitoring.FailThreshold <= 0 {
		d.Monitoring.FailThreshold = 5 // 默认失败5次告警
	}

	// 补全降级配置
	if d.Fallback == nil {
		d.Fallback = &FallbackOptions{}
	}
	if d.Fallback.RedisDownAction == "" {
		d.Fallback.RedisDownAction = "fail" // 默认Redis不可用时返回失败
	}
	if d.Fallback.MaxQueueSize <= 0 {
		d.Fallback.MaxQueueSize = 1000 // 默认队列大小1000
	}

	// 补全业务级配置（继承全局默认值）
	if d.Business == nil {
		d.Business = make(map[string]*BusinessLockOptions)
	}
	for _, business := range d.Business {
		// 业务重试配置继承全局默认（如果未配置）
		if business.Retry == nil {
			business.Retry = d.DefaultRetry
		} else {
			// 补全业务重试的缺失值
			if business.Retry.MaxCount <= 0 {
				business.Retry.MaxCount = d.DefaultRetry.MaxCount
			}
			if business.Retry.Interval <= 0 {
				business.Retry.Interval = d.DefaultRetry.Interval
			}
			if business.Retry.BackoffType == "" {
				business.Retry.BackoffType = d.DefaultRetry.BackoffType
			}
		}
		// 业务超时未配置时使用全局默认
		if business.Timeout <= 0 {
			business.Timeout = d.DefaultTimeout
		}
	}
}

// Validate 验证配置的有效性
// 作用：检查所有配置项是否符合规则，返回所有错误（不中断）
func (d *DistributedLockOptions) Validate() []error {
	var errors []error

	// 验证默认超时时间
	if d.DefaultTimeout <= 0 {
		errors = append(errors, fmt.Errorf("分布式锁默认超时时间必须大于0"))
	}

	// 验证全局重试配置
	if d.DefaultRetry != nil {
		if d.DefaultRetry.MaxCount < 0 {
			errors = append(errors, fmt.Errorf("默认重试次数不能为负数"))
		}
		if d.DefaultRetry.MaxCount > 0 && d.DefaultRetry.Interval <= 0 {
			errors = append(errors, fmt.Errorf("重试次数>0时，重试间隔必须大于0"))
		}
		if d.DefaultRetry.BackoffType != "fixed" && d.DefaultRetry.BackoffType != "exponential" {
			errors = append(errors, fmt.Errorf("重试策略必须为'fixed'或'exponential'"))
		}
	}

	// 验证Redis锁配置
	if d.Redis != nil {
		if d.Redis.DB < 0 {
			errors = append(errors, fmt.Errorf("Redis数据库索引不能为负数"))
		}
		if d.Redis.KeepAlive && d.Redis.KeepAliveInterval <= 0 {
			errors = append(errors, fmt.Errorf("启用锁续约时，续约间隔必须大于0"))
		}
	}

	// 验证监控配置
	if d.Monitoring != nil {
		if d.Monitoring.SlowThreshold <= 0 {
			errors = append(errors, fmt.Errorf("慢锁阈值必须大于0"))
		}
		if d.Monitoring.FailThreshold < 0 {
			errors = append(errors, fmt.Errorf("失败次数阈值不能为负数"))
		}
	}

	// 验证降级配置
	if d.Fallback != nil {
		if d.Fallback.RedisDownAction != "fail" && d.Fallback.RedisDownAction != "skip" {
			errors = append(errors, fmt.Errorf("Redis不可用时动作必须为'fail'或'skip'"))
		}
		if d.Fallback.MaxQueueSize < 0 {
			errors = append(errors, fmt.Errorf("降级队列大小不能为负数"))
		}
	}

	// 验证业务级配置
	for name, business := range d.Business {
		if business.Timeout < 0 {
			errors = append(errors, fmt.Errorf("业务[%s]锁超时时间不能为负数", name))
		}
		if business.Retry != nil && business.Retry.MaxCount < 0 {
			errors = append(errors, fmt.Errorf("业务[%s]重试次数不能为负数", name))
		}
	}

	return errors
}

// AddFlags 将配置项添加为命令行参数
// 作用：支持通过命令行参数（如--distributed-lock.enabled=true）覆盖配置文件
func (d *DistributedLockOptions) AddFlags(fs *pflag.FlagSet) {
	// 基础配置
	fs.BoolVar(&d.Enabled, "distributed-lock.enabled", d.Enabled, "是否启用分布式锁")
	fs.DurationVar(&d.DefaultTimeout, "distributed-lock.default-timeout", d.DefaultTimeout,
		"分布式锁默认超时时间（如5s、1m）")

	// 默认重试配置
	if d.DefaultRetry == nil {
		d.DefaultRetry = NewRetryOptions()
	}
	fs.IntVar(&d.DefaultRetry.MaxCount, "distributed-lock.default-retry.max-count", d.DefaultRetry.MaxCount,
		"获取锁失败时的默认最大重试次数")
	fs.DurationVar(&d.DefaultRetry.Interval, "distributed-lock.default-retry.interval", d.DefaultRetry.Interval,
		"获取锁失败时的默认重试间隔（如100ms）")
	fs.StringVar(&d.DefaultRetry.BackoffType, "distributed-lock.default-retry.backoff-type", d.DefaultRetry.BackoffType,
		"重试策略（fixed=固定间隔/exponential=指数退避）")

	// Redis锁配置
	if d.Redis == nil {
		d.Redis = NewRedisLockOptions()
	}
	fs.StringVar(&d.Redis.KeyPrefix, "distributed-lock.redis.key-prefix", d.Redis.KeyPrefix,
		"分布式锁键的统一前缀（避免键名冲突）")
	fs.IntVar(&d.Redis.DB, "distributed-lock.redis.db", d.Redis.DB,
		"分布式锁使用的Redis数据库索引")
	fs.BoolVar(&d.Redis.KeepAlive, "distributed-lock.redis.keep-alive", d.Redis.KeepAlive,
		"是否启用锁续约（防止业务未完成但锁已超时）")
	fs.DurationVar(&d.Redis.KeepAliveInterval, "distributed-lock.redis.keep-alive-interval", d.Redis.KeepAliveInterval,
		"锁续约间隔时间（建议为超时时间的1/3）")

	// 监控配置
	if d.Monitoring == nil {
		d.Monitoring = NewMonitoringOptions()
	}
	fs.BoolVar(&d.Monitoring.Enabled, "distributed-lock.monitoring.enabled", d.Monitoring.Enabled,
		"是否启用分布式锁监控告警")
	fs.DurationVar(&d.Monitoring.SlowThreshold, "distributed-lock.monitoring.slow-threshold", d.Monitoring.SlowThreshold,
		"慢锁阈值（超过此时长则告警，如1s）")
	fs.IntVar(&d.Monitoring.FailThreshold, "distributed-lock.monitoring.fail-threshold", d.Monitoring.FailThreshold,
		"单位时间内锁操作失败次数阈值（超过则告警）")

	// 降级配置
	if d.Fallback == nil {
		d.Fallback = NewFallbackOptions()
	}
	fs.BoolVar(&d.Fallback.Enabled, "distributed-lock.fallback.enabled", d.Fallback.Enabled,
		"是否启用分布式锁容错降级机制")
	fs.StringVar(&d.Fallback.RedisDownAction, "distributed-lock.fallback.redis-down-action", d.Fallback.RedisDownAction,
		"Redis不可用时的处理动作（fail=返回失败/skip=跳过锁）")
	fs.IntVar(&d.Fallback.MaxQueueSize, "distributed-lock.fallback.max-queue-size", d.Fallback.MaxQueueSize,
		"降级时的请求队列最大长度")
}

// 辅助函数：创建默认子配置实例（避免AddFlags时出现nil）
func NewRetryOptions() *RetryOptions {
	return &RetryOptions{
		MaxCount:    3,
		Interval:    100 * time.Millisecond,
		BackoffType: "fixed",
	}
}

func NewRedisLockOptions() *RedisLockOptions {
	return &RedisLockOptions{
		KeyPrefix:         "distributed:lock:",
		DB:                0,
		KeepAlive:         true,
		KeepAliveInterval: 2 * time.Second,
	}
}

func NewMonitoringOptions() *MonitoringOptions {
	return &MonitoringOptions{
		Enabled:       true,
		SlowThreshold: 1 * time.Second,
		FailThreshold: 5,
	}
}

func NewFallbackOptions() *FallbackOptions {
	return &FallbackOptions{
		Enabled:         true,
		RedisDownAction: "fail",
		MaxQueueSize:    1000,
	}
}
