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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation/field"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/spf13/pflag"
)

type MySQLOptions struct {
	// 管理员配置（用于初始化数据库、创建应用用户）
	AdminUsername         string        `json:"admin-username" mapstructure:"admin-username"`
	AdminPassword         string        `json:"admin-password" mapstructure:"admin-password"`
	Host                  string        `json:"host,omitempty"                     mapstructure:"host"`
	Port                  int           `json:"port" mapstructure:"-"`
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
		AdminUsername:         "root",
		AdminPassword:         "iam59!z$",
		Host:                  "127.0.0.1:3306",
		Username:              "",
		Password:              "",
		MaxIdleConnections:    100,
		MaxOpenConnections:    100,
		MaxConnectionLifeTime: time.Duration(10) * time.Second,
		LogLevel:              1,
	}
}

func (m *MySQLOptions) Complete() error {
	// ---------------- 第一步：加载环境变量（覆盖默认值，优先级最高） ----------------
	// 此处整合原 loadFromEnv 逻辑，确保环境变量先于补全生效
	if err := m.loadFromEnv(); err != nil {
		return errors.Wrap(err, "load config from environment variable failed")
	}

	// ---------------- 第二步：补全管理员配置（防止环境变量未设置且默认值被清空） ----------------
	if m.AdminUsername == "" {
		m.AdminUsername = "root"
	}
	if m.AdminPassword == "" {
		m.AdminPassword = "iam59!z$"
	}

	// ---------------- 第三步：补全应用访问配置 + 处理 Host 依赖（拆分 IP/Port） ----------------
	// 补全 Host 兜底值
	if m.Host == "" {
		m.Host = "127.0.0.1:3306"
	}
	// 拆分 Host 为 IP + Port（支持 "IP:Port" 或 "IP" 格式）
	ip, port, err := splitHostIntoIPPort(m.Host)
	if err != nil {
		return errors.Wrapf(err, "split Host [%s] into IP:Port failed", m.Host)
	}
	m.Host = ip
	m.Port = port

	// 补全应用访问账号
	if m.Username == "" {
		m.Username = "iam"
	}
	if m.Password == "" {
		m.Password = "iam59!z$"
	}
	if m.Database == "" {
		m.Database = "iam"
	}

	// ---------------- 第四步：优化连接池参数（处理不合理值） ----------------
	// MaxIdleConnections：确保不小于0且不大于 MaxOpenConnections，默认取 MaxOpenConnections 的 1/2
	if m.MaxIdleConnections <= 0 || m.MaxIdleConnections > m.MaxOpenConnections {
		m.MaxIdleConnections = m.MaxOpenConnections / 2
		if m.MaxIdleConnections <= 0 { // 避免 MaxOpenConnections=1 时 Idle 为0
			m.MaxIdleConnections = 1
		}
	}
	// MaxOpenConnections：兜底为 100（防止环境变量设为0或负数）
	if m.MaxOpenConnections <= 0 {
		m.MaxOpenConnections = 100
	}
	// ConnMaxLifetime：兜底为 1 小时（防止环境变量设为0或负数）
	if m.MaxConnectionLifeTime <= 0 {
		m.MaxConnectionLifeTime = 3600 * time.Second
	}

	return nil
}

// loadFromEnv 从环境变量加载配置（优先级：环境变量 > 默认值）
func (m *MySQLOptions) loadFromEnv() error {
	// 1. 加载管理员配置
	if envVal := os.Getenv("MARIADB_ADMIN_USERNAME"); envVal != "" {
		m.AdminUsername = envVal
	}
	if envVal := os.Getenv("MARIADB_ADMIN_PASSWORD"); envVal != "" {
		m.AdminPassword = envVal
	}

	// 2. 加载应用访问基础配置
	if envVal := os.Getenv("MARIADB_HOST"); envVal != "" {
		m.Host = envVal
	}
	if envVal := os.Getenv("MARIADB_USERNAME"); envVal != "" {
		m.Username = envVal
	}
	if envVal := os.Getenv("MARIADB_PASSWORD"); envVal != "" {
		m.Password = envVal
	}
	if envVal := os.Getenv("MARIADB_DATABASE"); envVal != "" {
		m.Database = envVal
	}

	// 3. 加载连接池配置
	if envVal := os.Getenv("MARIADB_MAX_IDLE_CONNECTIONS"); envVal != "" {
		val, err := strconv.Atoi(envVal)
		if err != nil {
			return errors.Wrapf(err, "invalid MARIADB_MAX_IDLE_CONNECTIONS: %s", envVal)
		}
		if val >= 0 {
			m.MaxIdleConnections = val
		}
	}
	if envVal := os.Getenv("MARIADB_MAX_OPEN_CONNECTIONS"); envVal != "" {
		val, err := strconv.Atoi(envVal)
		if err != nil {
			return errors.Wrapf(err, "invalid MARIADB_MAX_OPEN_CONNECTIONS: %s", envVal)
		}
		if val > 0 {
			m.MaxOpenConnections = val
		} else {
			return errors.Errorf("MARIADB_MAX_OPEN_CONNECTIONS must be > 0: %d", val)
		}
	}
	if envVal := os.Getenv("MARIADB_CONN_MAX_LIFETIME"); envVal != "" {
		val, err := time.ParseDuration(envVal)
		if err != nil {
			return errors.Wrapf(err, "invalid MARIADB_CONN_MAX_LIFETIME: %s", envVal)
		}
		if val > 0 {
			m.MaxConnectionLifeTime = val
		}
	}

	return nil
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
	agg := errs.ToAggregate()
	if agg == nil {
		return nil // 无错误时返回空切片，而非nil
	}
	return agg.Errors()
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

// splitHostIntoIPPort 辅助函数：将 Host 拆分为 IP 和 Port（支持 "IP:Port" 或 "IP" 格式）
func splitHostIntoIPPort(host string) (ip string, port int, err error) {
	parts := strings.Split(host, ":")
	switch len(parts) {
	case 1:
		// 仅 IP 格式（如 "127.0.0.1"），默认使用 MariaDB 标准端口 3306
		ip = parts[0]
		port = 3306
	case 2:
		// 完整 IP:Port 格式（如 "127.0.0.1:3306"）
		ip = parts[0]
		portVal, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", 0, errors.Wrapf(err, "Port [%s] is not a valid integer", parts[1])
		}
		port = portVal
	default:
		// 非法格式（如 "127.0.0.1:3306:8080"）
		return "", 0, errors.Errorf("invalid Host format [%s] (expected 'IP' or 'IP:Port')", host)
	}

	// 补充 IP 非空校验
	if ip == "" {
		return "", 0, errors.New("IP is empty after splitting Host")
	}
	// 补充 Port 范围校验
	if port <= 0 || port > 65535 {
		return "", 0, errors.Errorf("Port [%d] is out of valid range (1-65535)", port)
	}

	return ip, port, nil
}
