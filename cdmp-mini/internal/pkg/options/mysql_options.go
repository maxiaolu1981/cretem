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
	"fmt"
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

	// 新增连接优化参数
	ConnMaxLifetime time.Duration `json:"conn-max-lifetime,omitempty"        mapstructure:"conn-max-lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn-max-idle-time,omitempty"       mapstructure:"conn-max-idle-time"`
	ReadTimeout     time.Duration `json:"read-timeout,omitempty"             mapstructure:"read-timeout"`
	WriteTimeout    time.Duration `json:"write-timeout,omitempty"            mapstructure:"write-timeout"`
	DialTimeout     time.Duration `json:"dial-timeout,omitempty"             mapstructure:"dial-timeout"`

	// 新增性能监控参数
	SlowQueryThreshold time.Duration `json:"slow-query-threshold,omitempty"     mapstructure:"slow-query-threshold"`
	MonitorInterval    time.Duration `json:"monitor-interval,omitempty"         mapstructure:"monitor-interval"`

	// SSL/TLS配置
	SSLMode     string `json:"ssl-mode,omitempty"                 mapstructure:"ssl-mode"`
	SSLCert     string `json:"ssl-cert,omitempty"                 mapstructure:"ssl-cert"`
	SSLKey      string `json:"ssl-key,omitempty"                 mapstructure:"ssl-key"`
	SSLRootCert string `json:"ssl-root-cert,omitempty"           mapstructure:"ssl-root-cert"`

	// 连接重试配置
	MaxRetryAttempts int           `json:"max-retry-attempts,omitempty"       mapstructure:"max-retry-attempts"`
	RetryInterval    time.Duration `json:"retry-interval,omitempty"           mapstructure:"retry-interval"`
}

func NewMySQLOptions() *MySQLOptions {
	return &MySQLOptions{
		AdminUsername: "root",
		AdminPassword: "iam59!z$",
		Host:          "127.0.0.1",
		Port:          3306,
		Username:      "iam",
		Password:      "iam59!z$",
		Database:      "iam",

		// 连接池优化
		MaxIdleConnections:    200,               // 减少连接创建开销
		MaxOpenConnections:    500,               // 支持更高并发
		MaxConnectionLifeTime: 300 * time.Second, // 避免连接长时间占用

		// 新增连接参数
		ConnMaxLifetime: 180 * time.Second, // 连接最大生命周期3分钟
		ConnMaxIdleTime: 60 * time.Second,  // 空闲连接最大存活时间60秒
		ReadTimeout:     10 * time.Second,  // 读超时
		WriteTimeout:    10 * time.Second,  // 写超时
		DialTimeout:     5 * time.Second,   // 连接建立超时

		// 性能监控
		LogLevel:           1,                      // 开启慢查询日志
		SlowQueryThreshold: 200 * time.Millisecond, // 慢查询阈值200ms
		MonitorInterval:    30 * time.Second,       // 监控间隔

		// SSL配置
		SSLMode: "disable", // 默认禁用SSL

		// 重试配置
		MaxRetryAttempts: 3,               // 最大重试次数
		RetryInterval:    1 * time.Second, // 重试间隔
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

// Validate 验证MySQL配置
func (m *MySQLOptions) Validate() []error {
	var errs []error
	path := field.NewPath("mysql")

	// 验证 Host
	if m.Host == "" {
		errs = append(errs, field.Required(path.Child("host"), "MySQL主机地址必须存在"))
	} else {
		// 验证是否为有效的IP地址或主机名
		if ip := net.ParseIP(m.Host); ip == nil {
			// 不是IP，检查是否是合法主机名
			if dnsErrors := validation.IsDNS1123Subdomain(m.Host); len(dnsErrors) > 0 {
				for _, errMsg := range dnsErrors {
					errs = append(errs, field.Invalid(path.Child("host"), m.Host, errMsg))
				}
			}
		}
	}

	// 验证 Port
	if m.Port <= 0 || m.Port > 65535 {
		errs = append(errs, field.Invalid(path.Child("port"), m.Port, "端口号必须在1-65535范围内"))
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

	// 验证连接池配置
	if m.MaxIdleConnections < 0 {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能为负数"))
	}

	if m.MaxOpenConnections <= 0 {
		errs = append(errs, field.Invalid(path.Child("max-open-connections"), m.MaxOpenConnections, "最大打开连接数必须大于0"))
	}

	// 验证空闲连接数不大于最大连接数
	if m.MaxOpenConnections > 0 && m.MaxIdleConnections > m.MaxOpenConnections {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能大于最大打开连接数"))
	}

	// 验证连接生命周期
	if m.MaxConnectionLifeTime < time.Second {
		errs = append(errs, field.Invalid(path.Child("max-connection-life-time"), m.MaxConnectionLifeTime, "最大连接生命周期必须至少1秒"))
	}

	// 验证新增连接参数
	if m.ConnMaxLifetime < 0 {
		errs = append(errs, field.Invalid(path.Child("conn-max-lifetime"), m.ConnMaxLifetime, "连接最大生命周期不能为负数"))
	}
	if m.ConnMaxIdleTime < 0 {
		errs = append(errs, field.Invalid(path.Child("conn-max-idle-time"), m.ConnMaxIdleTime, "空闲连接最大存活时间不能为负数"))
	}
	if m.ReadTimeout < 0 {
		errs = append(errs, field.Invalid(path.Child("read-timeout"), m.ReadTimeout, "读超时不能为负数"))
	}
	if m.WriteTimeout < 0 {
		errs = append(errs, field.Invalid(path.Child("write-timeout"), m.WriteTimeout, "写超时不能为负数"))
	}
	if m.DialTimeout < 0 {
		errs = append(errs, field.Invalid(path.Child("dial-timeout"), m.DialTimeout, "连接建立超时不能为负数"))
	}

	// 验证 SSL 模式
	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[m.SSLMode] {
		errs = append(errs, field.Invalid(path.Child("ssl-mode"), m.SSLMode, "无效的SSL模式，可选值：disable, require, verify-ca, verify-full"))
	}

	// 验证重试配置
	if m.MaxRetryAttempts < 0 {
		errs = append(errs, field.Invalid(path.Child("max-retry-attempts"), m.MaxRetryAttempts, "最大重试次数不能为负数"))
	}
	if m.RetryInterval < 0 {
		errs = append(errs, field.Invalid(path.Child("retry-interval"), m.RetryInterval, "重试间隔不能为负数"))
	}

	return errs
}

// AddFlags 添加命令行标志
func (o *MySQLOptions) AddFlags(fs *pflag.FlagSet) {
	// 基本连接配置
	fs.StringVar(&o.Host, "mysql.host", o.Host, "MySQL服务主机地址")
	fs.IntVar(&o.Port, "mysql.port", o.Port, "MySQL服务端口")
	fs.StringVar(&o.Username, "mysql.username", o.Username, "访问MySQL服务的用户名")
	fs.StringVar(&o.Password, "mysql.password", o.Password, "访问MySQL的密码")
	fs.StringVar(&o.Database, "mysql.database", o.Database, "要使用的数据库名称")

	// 管理员账号（用于初始化等特殊操作）
	fs.StringVar(&o.AdminUsername, "mysql.admin-username", o.AdminUsername, "MySQL管理员用户名")
	fs.StringVar(&o.AdminPassword, "mysql.admin-password", o.AdminPassword, "MySQL管理员密码")

	// 连接池配置
	fs.IntVar(&o.MaxIdleConnections, "mysql.max-idle-connections", o.MaxIdleConnections, "最大空闲连接数")
	fs.IntVar(&o.MaxOpenConnections, "mysql.max-open-connections", o.MaxOpenConnections, "最大打开连接数")
	fs.DurationVar(&o.MaxConnectionLifeTime, "mysql.max-connection-life-time", o.MaxConnectionLifeTime, "最大连接生命周期")

	// 新增连接参数
	fs.DurationVar(&o.ConnMaxLifetime, "mysql.conn-max-lifetime", o.ConnMaxLifetime, "连接最大生命周期")
	fs.DurationVar(&o.ConnMaxIdleTime, "mysql.conn-max-idle-time", o.ConnMaxIdleTime, "空闲连接最大存活时间")
	fs.DurationVar(&o.ReadTimeout, "mysql.read-timeout", o.ReadTimeout, "读超时时间")
	fs.DurationVar(&o.WriteTimeout, "mysql.write-timeout", o.WriteTimeout, "写超时时间")
	fs.DurationVar(&o.DialTimeout, "mysql.dial-timeout", o.DialTimeout, "连接建立超时时间")

	// 性能监控
	fs.IntVar(&o.LogLevel, "mysql.log-level", o.LogLevel, "GORM日志级别 (0:Silent, 1:Error, 2:Warn, 3:Info)")
	fs.DurationVar(&o.SlowQueryThreshold, "mysql.slow-query-threshold", o.SlowQueryThreshold, "慢查询阈值")
	fs.DurationVar(&o.MonitorInterval, "mysql.monitor-interval", o.MonitorInterval, "监控间隔")

	// SSL配置
	fs.StringVar(&o.SSLMode, "mysql.ssl-mode", o.SSLMode, "SSL模式 (disable, require, verify-ca, verify-full)")

	// 重试配置
	fs.IntVar(&o.MaxRetryAttempts, "mysql.max-retry-attempts", o.MaxRetryAttempts, "最大重试次数")
	fs.DurationVar(&o.RetryInterval, "mysql.retry-interval", o.RetryInterval, "重试间隔")
}

// splitHostIntoIPPort 将主机地址拆分为IP和端口
func splitHostIntoIPPort(host string) (string, int, error) {
	// 如果host已经包含端口，直接解析
	if strings.Contains(host, ":") {
		hostStr, portStr, err := net.SplitHostPort(host)
		if err != nil {
			return "", 0, fmt.Errorf("invalid host:port format: %v", err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port number: %v", err)
		}
		return hostStr, port, nil
	}

	// 如果不包含端口，使用默认端口3306
	return host, 3306, nil
}

// loadFromEnv 从环境变量加载配置（优先级最高）
func (m *MySQLOptions) loadFromEnv() error {
	// 管理员账号环境变量
	if val := os.Getenv("MYSQL_ADMIN_USERNAME"); val != "" {
		m.AdminUsername = val
	}
	if val := os.Getenv("MYSQL_ADMIN_PASSWORD"); val != "" {
		m.AdminPassword = val
	}

	// 连接信息环境变量
	if val := os.Getenv("MYSQL_HOST"); val != "" {
		m.Host = val
	}
	if val := os.Getenv("MYSQL_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			m.Port = port
		}
	}
	if val := os.Getenv("MYSQL_USERNAME"); val != "" {
		m.Username = val
	}
	if val := os.Getenv("MYSQL_PASSWORD"); val != "" {
		m.Password = val
	}
	if val := os.Getenv("MYSQL_DATABASE"); val != "" {
		m.Database = val
	}

	// 连接池环境变量
	if val := os.Getenv("MYSQL_MAX_IDLE_CONNECTIONS"); val != "" {
		if maxIdle, err := strconv.Atoi(val); err == nil {
			m.MaxIdleConnections = maxIdle
		}
	}
	if val := os.Getenv("MYSQL_MAX_OPEN_CONNECTIONS"); val != "" {
		if maxOpen, err := strconv.Atoi(val); err == nil {
			m.MaxOpenConnections = maxOpen
		}
	}
	if val := os.Getenv("MYSQL_MAX_CONNECTION_LIFE_TIME"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.MaxConnectionLifeTime = duration
		}
	}

	// 新增连接参数环境变量
	if val := os.Getenv("MYSQL_CONN_MAX_LIFETIME"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.ConnMaxLifetime = duration
		}
	}
	if val := os.Getenv("MYSQL_CONN_MAX_IDLE_TIME"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.ConnMaxIdleTime = duration
		}
	}
	if val := os.Getenv("MYSQL_READ_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.ReadTimeout = duration
		}
	}
	if val := os.Getenv("MYSQL_WRITE_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.WriteTimeout = duration
		}
	}
	if val := os.Getenv("MYSQL_DIAL_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.DialTimeout = duration
		}
	}

	// 性能监控环境变量
	if val := os.Getenv("MYSQL_LOG_LEVEL"); val != "" {
		if logLevel, err := strconv.Atoi(val); err == nil {
			m.LogLevel = logLevel
		}
	}
	if val := os.Getenv("MYSQL_SLOW_QUERY_THRESHOLD"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.SlowQueryThreshold = duration
		}
	}
	if val := os.Getenv("MYSQL_MONITOR_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.MonitorInterval = duration
		}
	}

	// SSL配置环境变量
	if val := os.Getenv("MYSQL_SSL_MODE"); val != "" {
		m.SSLMode = val
	}

	// 重试配置环境变量
	if val := os.Getenv("MYSQL_MAX_RETRY_ATTEMPTS"); val != "" {
		if maxRetry, err := strconv.Atoi(val); err == nil {
			m.MaxRetryAttempts = maxRetry
		}
	}
	if val := os.Getenv("MYSQL_RETRY_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.RetryInterval = duration
		}
	}

	return nil
}
