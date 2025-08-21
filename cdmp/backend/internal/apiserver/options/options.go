// options包定义了IAM API服务器所有配置选项结果及管理办法,负责整合服务器运行所需要的各类参数(网络服务配置、数据库连接 认证信息 日志设置等),并且提供命令行参数解析 默认值填充 配置序列化等功能 是服务器配置系统的核心模块.
package options

import (
	cliflag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/json"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/idutil"
	"github.com/maxiaolu1981/cretem/nexuscore/log"

	genericoptions "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options"
)

// Options 整合了 IAM API 服务器运行所需的所有配置选项
type Options struct {
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"   mapstructure:"server"`   // 通用服务运行配置（如服务器模式、中间件等）
	GRPCOptions             *genericoptions.GRPCOptions            `json:"grpc"     mapstructure:"grpc"`     // gRPC 服务配置（端口、消息大小等）
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"` // 非安全服务配置（HTTP 端口、绑定地址等）
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"   mapstructure:"secure"`   // 安全服务配置（HTTPS 端口、TLS 证书等）
	MySQLOptions            *genericoptions.MySQLOptions           `json:"mysql"    mapstructure:"mysql"`    // MySQL 数据库连接配置
	RedisOptions            *genericoptions.RedisOptions           `json:"redis"    mapstructure:"redis"`    // Redis 缓存连接配置
	JwtOptions              *genericoptions.JwtOptions             `json:"jwt"      mapstructure:"jwt"`      // JWT 认证配置（密钥、超时时间等）
	Log                     *log.Options                           `json:"log"      mapstructure:"log"`      // 日志配置（级别、输出路径等）
	FeatureOptions          *genericoptions.FeatureOptions         `json:"feature"  mapstructure:"feature"`  // 功能开关配置（如指标、性能分析等）
}

// NewOptions 创建一个带有默认参数的 Options 实例
func NewOptions() *Options {
	o := Options{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),       // 初始化通用服务配置
		GRPCOptions:             genericoptions.NewGRPCOptions(),            // 初始化 gRPC 配置
		InsecureServing:         genericoptions.NewInsecureServingOptions(), // 初始化非安全服务配置
		SecureServing:           genericoptions.NewSecureServingOptions(),   // 初始化安全服务配置
		MySQLOptions:            genericoptions.NewMySQLOptions(),           // 初始化 MySQL 配置
		RedisOptions:            genericoptions.NewRedisOptions(),           // 初始化 Redis 配置
		JwtOptions:              genericoptions.NewJwtOptions(),             // 初始化 JWT 配置
		Log:                     log.NewOptions(),                           // 初始化日志配置
		FeatureOptions:          genericoptions.NewFeatureOptions(),         // 初始化功能开关配置
	}

	return &o
}

// Flags 按分组生成命令行参数集合，用于命令行解析
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	// 为不同配置项添加对应的命令行参数组
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("generic"))  // 通用服务参数组
	o.JwtOptions.AddFlags(fss.FlagSet("jwt"))                   // JWT 配置参数组
	o.GRPCOptions.AddFlags(fss.FlagSet("grpc"))                 // gRPC 配置参数组
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))               // MySQL 配置参数组
	o.RedisOptions.AddFlags(fss.FlagSet("redis"))               // Redis 配置参数组
	o.FeatureOptions.AddFlags(fss.FlagSet("features"))          // 功能开关参数组
	o.InsecureServing.AddFlags(fss.FlagSet("insecure serving")) // 非安全服务参数组
	o.SecureServing.AddFlags(fss.FlagSet("secure serving"))     // 安全服务参数组
	o.Log.AddFlags(fss.FlagSet("logs"))                         // 日志配置参数组

	return fss
}

// String 将配置序列化为 JSON 字符串，用于调试和日志输出
func (o *Options) String() string {
	data, _ := json.Marshal(o) // 忽略序列化错误，仅用于展示

	return string(data)
}

// Complete 补全配置默认值，确保关键参数不为空
func (o *Options) Complete() error {
	// 若 JWT 私钥未设置，自动生成随机密钥
	if o.JwtOptions.Key == "" {
		o.JwtOptions.Key = idutil.NewSecretKey()
	}

	// 补全安全服务配置的默认值（如证书路径等）
	return o.SecureServing.Complete()
}
