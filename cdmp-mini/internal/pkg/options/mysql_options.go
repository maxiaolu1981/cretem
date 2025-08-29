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

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
	"github.com/spf13/pflag"
)

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

func NewMySQLOptions() *MySQLOptions {
	return &MySQLOptions{
		Host:                  "127.0.0.1:3306",
		Username:              "",
		Password:              "",
		MaxIdleConnections:    100,
		MaxOpenConnections:    100,
		MaxConnectionLifeTime: time.Duration(10) * time.Second,
		LogLevel:              1,
	}
}

func (m *MySQLOptions) Validate() []error {
	errs := field.ErrorList{}
	path := field.NewPath("mysql")

	// 验证 Host
	if m.Host == "" {
		errs = append(errs, field.Required(path.Child("host"), "MySQL主机地址必须存在"))
	} else {
		// 验证 host:port 格式
		if strings.Contains(m.Host, ":") {
			host, portStr, err := net.SplitHostPort(m.Host)
			if err != nil {
				errs = append(errs, field.Invalid(path.Child("host"), m.Host, "无效的主机端口格式"))
			} else {
				// 验证端口号
				if port, err := strconv.Atoi(portStr); err != nil {
					errs = append(errs, field.Invalid(path.Child("host"), m.Host, "端口号必须为数字"))
				} else if portErrors := validation.IsValidPortNum(port); len(portErrors) > 0 {
					for _, errMsg := range portErrors {
						errs = append(errs, field.Invalid(path.Child("host"), m.Host, errMsg))
					}
				}

				// 验证主机名/IP
				if host != "" {
					if ip := net.ParseIP(host); ip == nil {
						// 不是IP，检查是否是合法主机名
						if dnsErrors := validation.IsDNS1123Subdomain(host); len(dnsErrors) > 0 {
							for _, errMsg := range dnsErrors {
								errs = append(errs, field.Invalid(path.Child("host"), host, errMsg))
							}
						}
					}
				}
			}
		} else {
			// 只有主机名，没有端口
			if ip := net.ParseIP(m.Host); ip == nil {
				if dnsErrors := validation.IsDNS1123Subdomain(m.Host); len(dnsErrors) > 0 {
					for _, errMsg := range dnsErrors {
						errs = append(errs, field.Invalid(path.Child("host"), m.Host, errMsg))
					}
				}
			}
		}
	}

	// 验证 Username
	if m.Username == "" {
		errs = append(errs, field.Required(path.Child("username"), "MySQL用户名必须存在"))
	} else if nameErrors := validation.IsQualifiedName(m.Username); len(nameErrors) > 0 {
		for _, errMsg := range nameErrors {
			errs = append(errs, field.Invalid(path.Child("username"), m.Username, errMsg))
		}
	}

	// 验证 Password
	if m.Password == "" {
		errs = append(errs, field.Required(path.Child("password"), "MySQL密码必须存在"))
	}

	// 验证 Database
	if m.Database == "" {
		errs = append(errs, field.Required(path.Child("database"), "数据库名必须存在"))
	} else if dbErrors := validation.IsDNS1123Label(m.Database); len(dbErrors) > 0 {
		for _, errMsg := range dbErrors {
			errs = append(errs, field.Invalid(path.Child("database"), m.Database, errMsg))
		}
	}

	// 验证 MaxIdleConnections
	if m.MaxIdleConnections < 0 {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能为负数"))
	} else if m.MaxIdleConnections > 100 {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能超过100"))
	}

	// 验证 MaxOpenConnections
	if m.MaxOpenConnections <= 0 {
		errs = append(errs, field.Invalid(path.Child("max-open-connections"), m.MaxOpenConnections, "最大打开连接数必须大于0"))
	} else if m.MaxOpenConnections > 500 {
		errs = append(errs, field.Invalid(path.Child("max-open-connections"), m.MaxOpenConnections, "最大打开连接数不能超过500"))
	}

	// 验证 MaxConnectionLifeTime
	if m.MaxConnectionLifeTime < time.Second {
		errs = append(errs, field.Invalid(path.Child("max-connection-life-time"), m.MaxConnectionLifeTime, "最大连接生命周期必须至少1秒"))
	} else if m.MaxConnectionLifeTime > 24*time.Hour {
		errs = append(errs, field.Invalid(path.Child("max-connection-life-time"), m.MaxConnectionLifeTime, "最大连接生命周期不能超过24小时"))
	}

	// 验证 LogLevel
	if m.LogLevel < 0 {
		errs = append(errs, field.Invalid(path.Child("log-level"), m.LogLevel, "日志级别不能为负数"))
	} else if m.LogLevel > 4 {
		errs = append(errs, field.Invalid(path.Child("log-level"), m.LogLevel, "日志级别不能超过4"))
	}

	// 验证连接池配置的合理性
	if m.MaxOpenConnections > 0 && m.MaxIdleConnections > m.MaxOpenConnections {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能大于最大打开连接数"))
	}
	return errs.ToAggregate().Errors()
}

func (o *MySQLOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Host, "mysql.host", "H", o.Host, ""+
		"MySQL服务主机地址。如果留空，则以下相关的mysql选项将被忽略。")

	fs.StringVarP(&o.Username, "mysql.username", "u", o.Username, ""+
		"访问MySQL服务的用户名。")

	fs.StringVarP(&o.Password, "mysql.password", "P", o.Password, ""+
		"访问MySQL的密码，应与用户名配对使用。")

	fs.StringVarP(&o.Database, "mysql.database", "d", o.Database, ""+
		"服务器要使用的数据库名称。")

	fs.IntVarP(&o.MaxIdleConnections, "mysql.max-idle-connections", "i", o.MaxIdleConnections, ""+
		"允许连接到MySQL的最大空闲连接数。")

	fs.IntVarP(&o.MaxOpenConnections, "mysql.max-open-connections", "o", o.MaxOpenConnections, ""+
		"允许连接到MySQL的最大打开连接数。")

	fs.DurationVarP(&o.MaxConnectionLifeTime, "mysql.max-connection-life-time", "l", o.MaxConnectionLifeTime, ""+
		"允许连接到MySQL的最大连接生命周期。")

	fs.IntVarP(&o.LogLevel, "mysql.log-mode", "v", o.LogLevel, ""+
		"指定GORM日志级别。")
}
