/*
这是一个用于 MySQL 数据库连接配置的 Go 语言选项包。

包摘要
这个 options 包提供了 MySQL 数据库连接的完整配置管理。

1. 核心结构体 MySQLOptions
包含 MySQL 连接的所有配置参数：

连接参数: Host, Username, Password, Database

连接池参数: MaxIdleConnections, MaxOpenConnections, MaxConnectionLifeTime

日志参数: LogLevel (Gorm 内部日志级别)

2. 主要功能
构造函数

NewMySQLOptions(): 创建带有合理默认值的配置实例

默认主机: 127.0.0.1:3306

默认用户: root

默认密码: iam59!z$

默认数据库: iam

连接池默认值已优化配置

命令行集成

AddFlags(): 支持通过命令行参数配置所有 MySQL 选项

配置验证

Validate(): 全面的配置验证，包括：

用户名和数据库名非空检查

密码强度验证（使用外部验证组件）

连接池参数合理性检查

日志级别范围验证（0-4）

参数间依赖关系验证（如空闲连接数不能大于最大连接数）

3. 特性
灵活的配置方式: 支持代码配置、命令行参数配置

智能验证: 当 Host 为空时跳过其他验证，支持可选配置

安全性: 密码字段使用 json:"-" 避免序列化泄露

生产就绪: 提供合理的连接池默认值和参数验证

4. 使用场景
用于需要 MySQL 数据库连接的应用程序，特别是：

微服务架构中的数据库配置管理

需要命令行参数配置的应用程序

需要严格参数验证的生产环境部署

5. 依赖组件
github.com/maxiaolu1981/cretem/nexuscore/component-base/validation: 密码强度验证

github.com/spf13/pflag: 命令行标志解析

这个包提供了完整、安全且易于使用的 MySQL 配置管理解决方案。
*/
package options

import "time"

type MySQLOptions struct {
	Host                  string        `json:"host,omitempty"                     mapstructure:"host"`
	Username              string        `json:"username,omitempty"                 mapstructure:"username"`
	Password              string        `json:"-"                                  mapstructure:"password"`
	Database              string        `json:"database"                           mapstructure:"database"`
	MaxIdleConnections    int           `json:"max-idle-connections,omitempty"     mapstructure:"max-idle-connections"`
	MaxOpenConnections    int           `json:"max-open-connections,omitempty"     mapstructure:"max-open-connections"`
	MaxConnectionLifeTime time.Duration `json:"max-connection-life-time,omitempty" mapstructure:"max-connection-life-time"`
	LogLevel              int           `json:"log-level"                          mapstructure:"log-level"`
}
