/*
该包定义了 IAM API 服务器的所有配置选项结构及管理方法，负责整合服务器运行所需的各类参数（如网络服务配置、数据库连接、认证信息、日志设置等），并提供命令行参数解析、默认值填充、配置序列化等功能，是服务器配置系统的核心模块。
函数流程详解
Options 结构体
作为配置选项的容器，聚合了服务器运行的所有关键配置，包括：
通用服务配置（GenericServerRunOptions）
gRPC 服务配置（GRPCOptions）
非安全 / 安全服务配置（InsecureServing/SecureServing）
数据库配置（MySQL、Redis）
JWT 认证配置（JwtOptions）
日志配置（Log）
功能开关配置（FeatureOptions）
每个字段通过 json 和 mapstructure 标签支持配置文件解析。
NewOptions 函数
功能：创建一个包含默认值的 Options 实例。
流程：初始化结构体中所有子配置项（调用各自的 NewXXXOptions 方法），确保每个配置都有合理的默认值（如默认端口、默认日志级别等）。
ApplyTo 函数
功能：将当前 Options 配置应用到服务器核心配置（server.Config）。
现状：当前实现为空（返回 nil），实际使用时需补充具体的配置映射逻辑，将 Options 中的字段赋值给服务器运行时配置。
Flags 函数
功能：为命令行解析生成结构化的参数集合。
流程：按配置类型分组（如 "generic"、"jwt"、"mysql" 等），调用各子配置的 AddFlags 方法，将参数添加到对应的命令行参数集（NamedFlagSets），方便外部解析命令行参数时按组展示和处理。
String 函数
功能：将 Options 配置序列化为 JSON 字符串，用于日志打印或配置调试。
流程：使用 json.Marshal 将结构体转换为 JSON 字节流，返回字符串形式（忽略序列化错误）。
Complete 函数
功能：补全配置默认值，确保关键配置项不为空。
流程：
若 JWT 私钥（JwtOptions.Key）未设置，则自动生成一个随机密钥（idutil.NewSecretKey()）。
调用 SecureServing.Complete() 补全安全服务配置的默认值（如证书路径等）。
返回补全过程中可能出现的错误。

*/
// options 包定义了 IAM API 服务器的所有配置选项，负责管理服务器运行所需的各类参数，
// 包括服务配置、数据库连接、认证信息、日志设置等，并提供配置解析和默认值处理功能。
package options

import (
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
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

// ApplyTo 将当前配置应用到服务器核心配置（server.Config）
// 注：当前实现为占位，实际使用时需补充配置映射逻辑
func (o *Options) ApplyTo(c *server.Config) error {
	return nil
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
