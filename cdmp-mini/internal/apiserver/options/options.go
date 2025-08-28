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
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

type Options struct {
	InsecureServingOptions *options.InsecureServingOptions
}

func (o *Options) Validate() []error {
	return nil
}

func (o *Options) Flags() (fss *cliFlag.NamedFlagSets) {
	o.InsecureServingOptions.AddFlags(fss.FlagSet("insecure serving"))
	return fss
}
