package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

type RedisOptions struct {
	Host                  string   `json:"host"                     mapstructure:"host"                     description:"Redis service host address"`
	Port                  int      `json:"port"`
	Addrs                 []string `json:"addrs"                    mapstructure:"addrs"`
	Username              string   `json:"username"                 mapstructure:"username"`
	Password              string   `json:"password"                 mapstructure:"password"`
	Database              int      `json:"database"                 mapstructure:"database"`
	MasterName            string   `json:"master-name"              mapstructure:"master-name"`
	MaxIdle               int      `json:"optimisation-max-idle"    mapstructure:"optimisation-max-idle"`
	MaxActive             int      `json:"optimisation-max-active"  mapstructure:"optimisation-max-active"`
	Timeout               int      `json:"timeout"                  mapstructure:"timeout"`
	EnableCluster         bool     `json:"enable-cluster"           mapstructure:"enable-cluster"`
	UseSSL                bool     `json:"use-ssl"                  mapstructure:"use-ssl"`
	SSLInsecureSkipVerify bool     `json:"ssl-insecure-skip-verify" mapstructure:"ssl-insecure-skip-verify"`
}

func NewRedisOptions() *RedisOptions {
	return &RedisOptions{
		Host:                  "127.0.0.1",
		Port:                  6379,
		Addrs:                 []string{},
		Username:              "",
		Password:              "",
		Database:              0,
		MasterName:            "",
		MaxIdle:               2000,
		MaxActive:             4000,
		Timeout:               0,
		EnableCluster:         false,
		UseSSL:                false,
		SSLInsecureSkipVerify: false,
	}
}

// Complete 补全Redis配置选项，处理默认值和依赖关系
func (r *RedisOptions) Complete() {
	// 处理地址信息 - 优先使用Addrs，没有则从Host和Port生成
	if len(r.Addrs) == 0 {
		host := r.Host
		if host == "" {
			host = "localhost" // 默认主机地址
		}

		port := r.Port
		if port == 0 {
			port = 6379 // Redis默认端口
		}

		r.Addrs = []string{fmt.Sprintf("%s:%d", host, port)}
	}

	// 连接池默认配置
	if r.MaxIdle <= 0 {
		r.MaxIdle = 10 // 默认最大空闲连接数
	}
	if r.MaxActive <= 0 {
		r.MaxActive = 100 // 默认最大活跃连接数
	}

	// 超时设置默认值
	if r.Timeout <= 0 {
		r.Timeout = 30 // 默认超时时间(秒)
	}
}

// Validate 验证Redis配置选项的有效性，返回所有验证错误
func (r *RedisOptions) Validate() []error {
	var errors []error

	// 验证地址配置 - 必须提供至少一个地址
	if len(r.Addrs) == 0 && (r.Host == "" && r.Port == 0) {
		errors = append(errors, fmt.Errorf("redis配置警告：未提供有效地址，需配置Addrs或Host/Port"))
	}

	// 如果提供了Host但没有Port，或者提供了Port但没有Host
	if (r.Host != "" && r.Port == 0) || (r.Host == "" && r.Port != 0) {
		errors = append(errors, fmt.Errorf("redis配置警告：Host和Port需同时配置或同时不配置"))
	}

	// 验证数据库索引有效性
	if r.Database < 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：数据库索引不能为负数"))
	}

	// 验证连接池配置有效性
	if r.MaxIdle < 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：最大空闲连接数不能为负数"))
	}
	if r.MaxActive < 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：最大活跃连接数不能为负数"))
	}

	// 验证超时配置有效性
	if r.Timeout < 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：超时时间不能为负数"))
	}

	// 集群模式验证
	if r.EnableCluster && len(r.Addrs) == 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：启用集群模式时必须配置Addrs"))
	}

	// 主从模式验证
	if r.MasterName != "" && len(r.Addrs) == 0 {
		errors = append(errors, fmt.Errorf("redis配置警告：配置主节点名称时必须配置Addrs"))
	}

	// SSL配置验证
	if r.SSLInsecureSkipVerify && !r.UseSSL {
		errors = append(errors, fmt.Errorf("redis配置警告：仅当UseSSL为true时才能设置SSLInsecureSkipVerify"))
	}

	return errors
}

// AddFlags 将Redis配置选项添加为命令行标志
func (r *RedisOptions) AddFlags(fs *pflag.FlagSet) {
	// 基础连接配置
	fs.StringVar(&r.Host, "redis.host", r.Host, "Redis service host address")
	fs.IntVar(&r.Port, "redis.port", r.Port, "Redis service port number")
	fs.StringSliceVar(&r.Addrs, "redis.addrs", r.Addrs, "List of Redis server addresses (used for cluster mode)")
	fs.StringVar(&r.Username, "redis.username", r.Username, "Username for Redis authentication")
	fs.StringVar(&r.Password, "redis.password", r.Password, "Password for Redis authentication")
	fs.IntVar(&r.Database, "redis.database", r.Database, "Redis database index to use")

	// 主从集群配置
	fs.StringVar(&r.MasterName, "redis.master-name", r.MasterName, "Name of the master node in sentinel mode")
	fs.BoolVar(&r.EnableCluster, "redis.enable-cluster", r.EnableCluster, "Enable Redis cluster mode")

	// 连接池优化配置
	fs.IntVar(&r.MaxIdle, "redis.optimisation-max-idle", r.MaxIdle, "Maximum number of idle connections in the pool")
	fs.IntVar(&r.MaxActive, "redis.optimisation-max-active", r.MaxActive, "Maximum number of active connections in the pool")
	fs.IntVar(&r.Timeout, "redis.timeout", r.Timeout, "Connection timeout in seconds")

	// SSL配置
	fs.BoolVar(&r.UseSSL, "redis.use-ssl", r.UseSSL, "Enable SSL for Redis connections")
	fs.BoolVar(&r.SSLInsecureSkipVerify, "redis.ssl-insecure-skip-verify", r.SSLInsecureSkipVerify, "Skip SSL certificate verification (not recommended for production)")
}

// AddFlags 将 MetaOptions 的参数添加到命令行标志集中
func (o *MetaOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil || fs == nil {
		return
	}

	// 确保指针字段已经初始化
	if o.ListOptions.TimeoutSeconds == nil {
		timeout := int64(60)
		o.ListOptions.TimeoutSeconds = &timeout
	}
	if o.ListOptions.Offset == nil {
		offset := int64(20)
		o.ListOptions.Offset = &offset
	}
	if o.ListOptions.Limit == nil {
		limit := int64(50)
		o.ListOptions.Limit = &limit
	}

	// ListOptions 相关标志
	fs.Int64Var(o.ListOptions.TimeoutSeconds, "timeout", *o.ListOptions.TimeoutSeconds, "查询超时时间（秒），0表示不超时")
	fs.Int64Var(o.ListOptions.Offset, "offset", *o.ListOptions.Offset, "分页偏移量，跳过前N条记录")
	fs.Int64Var(o.ListOptions.Limit, "limit", *o.ListOptions.Limit, "每页条数限制，最大100条")
	fs.StringVar(&o.ListOptions.LabelSelector, "selector", "", "标签选择器，用于筛选资源")
	fs.StringVar(&o.ListOptions.FieldSelector, "field-selector", "", "字段选择器，用于按字段筛选资源")

	// UpdateOptions 相关标志
	fs.StringSliceVar(&o.UpdateOptions.DryRun, "update-dry-run", nil, "更新操作的试运行模式，值为 All 开启试运行")

	// CreateOptions 相关标志
	fs.StringSliceVar(&o.CreateOptions.DryRun, "create-dry-run", nil, "创建操作的试运行模式，值为 All 开启试运行")

	// DeleteOptions 相关标志
	fs.BoolVar(&o.DeleteOptions.Unscoped, "unscoped", false, "是否不使用默认作用域（如包含软删除的记录）")

	// TypeMeta 相关标志（可选）
	fs.StringVar(&o.ListOptions.Kind, "kind", "", "资源类型，如 Deployment、Pod、Service 等")
	fs.StringVar(&o.ListOptions.APIVersion, "api-version", "v1", "API版本")
}
