/*
这个包是IAM项目的API服务器选项配置模块，负责定义和管理所有命令行标志和配置选项。以下是包摘要：
1.包功能
选项定义: 定义API服务器所有的配置选项结构
标志管理: 管理命令行标志的注册和分组
默认值设置: 提供各选项的默认值初始化
配置完整性: 确保配置的完整性和合理性
2.主要方法
NewOptions(): 创建带有默认参数的Options对象
Flags(): 返回分组的命令行标志集（NamedFlagSets）
Complete(): 完成选项的默认值设置和验证
ApplyTo(): 将选项应用到服务器配置（当前为空实现）
String(): 返回选项的JSON格式字符串表示
3.配置分组
generic: 通用服务器运行选项
jwt: JWT认证相关选项
grpc: gRPC服务选项
mysql: MySQL数据库选项
redis: Redis缓存选项
features: 功能特性选项
insecure serving: 非安全服务选项
secure serving: 安全服务选项\
logs: 日志配置选项
4.特色功能
自动密钥生成: 如果JWT密钥未设置，自动生成安全密钥
结构化分组: 使用NamedFlagSets进行逻辑分组管理
JSON序列化: 支持配置的JSON格式输出
模块化设计: 各选项模块独立，便于维护和扩展
这个包作为配置管理的核心，为IAM API服务器提供了完整的配置选项管理和命令行接口支持。
*/
package options

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

type Options struct {
	InsecureServingOptions *options.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	JwtOptions             *options.JwtOptions             `json:"jwt"      mapstructure:"jwt"`
	MysqlOptions           *options.MySQLOptions           `json:"mysql"    mapstructure:"mysql"`
	ServerRunOptions       *options.ServerRunOptions       `json:"server"   mapstructure:"server"`
	Log                    *log.Options                    `json:"log"      mapstructure:"log"`
	RedisOptions           *options.RedisOptions           `json:"redis"    mapstructure:"redis"`
	MetaOptions            *options.MetaOptions            `json:"metoptions" mapstructure:"metoptions"`
	KafkaOptions           *options.KafkaOptions           `json:"kafkaoptions" mapstructure:"kafkaoptions"`
}

func NewOptions() *Options {
	return &Options{
		InsecureServingOptions: options.NewInsecureServingOptions(),
		JwtOptions:             options.NewJwtOptions(),
		MysqlOptions:           options.NewMySQLOptions(),
		ServerRunOptions:       options.NewServerRunOptions(),
		Log:                    log.NewOptions(),
		RedisOptions:           options.NewRedisOptions(),
		MetaOptions:            options.NewMetaOptions(),
		KafkaOptions:           options.NewKafkaOptions(),
	}
}

func (o *Options) Complete() {
	o.InsecureServingOptions.Complete()
	o.JwtOptions.Complete()
	o.ServerRunOptions.Complete()
	o.MysqlOptions.Complete()
	o.Log.Complete()
	o.MetaOptions.Complete()
	o.KafkaOptions.Complete()

}

func (o *Options) Validate() []error {
	var errs []error
	errs = append(errs, o.InsecureServingOptions.Validate()...)
	errs = append(errs, o.JwtOptions.Validate()...)
	errs = append(errs, o.MysqlOptions.Validate()...)
	errs = append(errs, o.ServerRunOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	errs = append(errs, o.RedisOptions.Validate()...)
	errs = append(errs, o.MetaOptions.Validate()...)
	errs = append(errs, o.KafkaOptions.Validate()...)

	return errs
}

func (o *Options) Flags() (fss cliFlag.NamedFlagSets) {
	o.InsecureServingOptions.AddFlags(fss.FlagSet("insecure serving"))
	o.MysqlOptions.AddFlags(fss.FlagSet("mysql"))
	o.JwtOptions.AddFlags(fss.FlagSet("jwt"))
	o.ServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.Log.AddFlags(fss.FlagSet("log"))
	o.RedisOptions.AddFlags(fss.FlagSet("redis"))
	o.MetaOptions.AddFlags(fss.FlagSet("meta"))
	o.KafkaOptions.AddFlags(fss.FlagSet("kafka"))

	return fss
}
