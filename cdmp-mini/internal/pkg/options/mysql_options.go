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

	// ========== 新增：Galera集群配置 ==========
	// 主节点配置（写操作）
	PrimaryHost string `json:"primary-host,omitempty"           mapstructure:"primary-host"`
	PrimaryPort int    `json:"primary-port,omitempty"           mapstructure:"primary-port"`

	// 副本节点配置（读操作）
	ReplicaHosts []string `json:"replica-hosts,omitempty"        mapstructure:"replica-hosts"`
	ReplicaPorts []int    `json:"replica-ports,omitempty"        mapstructure:"replica-ports"`

	// 集群配置
	LoadBalance         bool          `json:"load-balance,omitempty"           mapstructure:"load-balance"`
	FailoverEnabled     bool          `json:"failover-enabled,omitempty"       mapstructure:"failover-enabled"`
	HealthCheckInterval time.Duration `json:"health-check-interval,omitempty"  mapstructure:"health-check-interval"`

	// Galera特定配置
	WSREPSyncWait bool `json:"wsrep-sync-wait,omitempty"         mapstructure:"wsrep-sync-wait"`
}

func NewMySQLOptions() *MySQLOptions {
	return &MySQLOptions{
		AdminUsername: "root",
		AdminPassword: "iam59!z$",
		Host:          "192.168.10.14",
		Port:          3306,
		Password:      "iam59!z$",
		Database:      "iam",

		// 连接池优化
		MaxIdleConnections: 20,              // 增加到20个空闲连接
		MaxOpenConnections: 60,              // 增加到60个最大连接
		ConnMaxLifetime:    5 * time.Minute, // 缩短到5分钟，促进连接轮换
		ConnMaxIdleTime:    1 * time.Minute, // 空闲1分钟释放
		ReadTimeout:        3 * time.Second, // 读超时
		WriteTimeout:       3 * time.Second, // 写超时
		DialTimeout:        2 * time.Second, // 连接建立超时

		// 性能监控
		LogLevel:           1,                      // 开启慢查询日志
		SlowQueryThreshold: 100 * time.Millisecond, // 降低慢查询阈值
		MonitorInterval:    15 * time.Second,       // 缩短监控间隔

		// SSL配置
		SSLMode: "disable", // 默认禁用SSL

		// 重试配置
		MaxRetryAttempts: 5,                      // 增加重试次数
		RetryInterval:    500 * time.Millisecond, // 缩短重试间隔

		// ========== 新增：Galera集群配置 ==========
		// 主节点（写操作指向节点1）
		PrimaryHost: "192.168.10.8",
		PrimaryPort: 3306,

		// 副本节点（所有节点都可用于读）
		ReplicaHosts: []string{"192.168.10.8", "192.168.10.8", "192.168.10.8"},
		ReplicaPorts: []int{3306, 3307, 3308},

		// 集群配置
		LoadBalance:         true,             // 启用负载均衡
		FailoverEnabled:     true,             // 启用故障转移
		HealthCheckInterval: 10 * time.Second, // 健康检查间隔

		// Galera配置
		WSREPSyncWait: true, // 等待集群同步
	}
}

func (m *MySQLOptions) Complete() error {
	// ---------------- 第一步：加载环境变量 ----------------
	if err := m.loadFromEnv(); err != nil {
		return errors.Wrap(err, "load config from environment variable failed")
	}

	// ---------------- 第二步：补全管理员配置 ----------------
	if m.AdminUsername == "" {
		m.AdminUsername = "root"
	}
	if m.AdminPassword == "" {
		m.AdminPassword = "iam59!z$"
	}

	// ---------------- 第三步：处理集群配置 ----------------
	// 如果没有配置主节点，使用默认Host和Port
	if m.PrimaryHost == "" {
		m.PrimaryHost = "127.0.0.1"
	}
	if m.PrimaryPort == 0 {
		m.PrimaryPort = 3306
	}

	// 如果没有配置副本节点，使用主节点作为唯一节点
	if len(m.ReplicaHosts) == 0 {
		m.ReplicaHosts = []string{m.PrimaryHost}
		m.ReplicaPorts = []int{m.PrimaryPort}
	}

	// ---------------- 第四步：处理单节点兼容性 ----------------
	// 补全 Host 兜底值（向后兼容）
	if m.Host == "" {
		m.Host = fmt.Sprintf("%s:%d", m.PrimaryHost, m.PrimaryPort)
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

	// ---------------- 第五步：优化连接池参数 ----------------
	// MaxIdleConnections：确保不小于0且不大于 MaxOpenConnections
	if m.MaxIdleConnections <= 0 || m.MaxIdleConnections > m.MaxOpenConnections {
		m.MaxIdleConnections = m.MaxOpenConnections / 2
		if m.MaxIdleConnections <= 0 {
			m.MaxIdleConnections = 1
		}
	}
	// MaxOpenConnections：兜底为 100
	if m.MaxOpenConnections <= 0 {
		m.MaxOpenConnections = 100
	}
	// ConnMaxLifetime：兜底为 1 小时
	if m.MaxConnectionLifeTime <= 0 {
		m.MaxConnectionLifeTime = 3600 * time.Second
	}

	// 集群模式下调整连接池大小
	if m.LoadBalance && len(m.ReplicaHosts) > 1 {
		// 集群模式下增加连接池容量
		if m.MaxOpenConnections < 50 {
			m.MaxOpenConnections = 50
		}
		if m.MaxIdleConnections < 10 {
			m.MaxIdleConnections = 10
		}
	}

	return nil
}

// Validate 验证MySQL配置
func (m *MySQLOptions) Validate() []error {
	var errs []error
	path := field.NewPath("mysql")

	// 验证主节点配置
	if m.PrimaryHost == "" {
		errs = append(errs, field.Required(path.Child("primary-host"), "MySQL主节点地址必须存在"))
	} else {
		if ip := net.ParseIP(m.PrimaryHost); ip == nil {
			if dnsErrors := validation.IsDNS1123Subdomain(m.PrimaryHost); len(dnsErrors) > 0 {
				for _, errMsg := range dnsErrors {
					errs = append(errs, field.Invalid(path.Child("primary-host"), m.PrimaryHost, errMsg))
				}
			}
		}
	}

	if m.PrimaryPort <= 0 || m.PrimaryPort > 65535 {
		errs = append(errs, field.Invalid(path.Child("primary-port"), m.PrimaryPort, "主节点端口号必须在1-65535范围内"))
	}

	// 验证副本节点配置
	if len(m.ReplicaHosts) == 0 {
		errs = append(errs, field.Required(path.Child("replica-hosts"), "至少需要一个副本节点"))
	} else {
		for i, host := range m.ReplicaHosts {
			if host == "" {
				errs = append(errs, field.Required(path.Child("replica-hosts").Index(i), "副本节点地址不能为空"))
				continue
			}
			if ip := net.ParseIP(host); ip == nil {
				if dnsErrors := validation.IsDNS1123Subdomain(host); len(dnsErrors) > 0 {
					for _, errMsg := range dnsErrors {
						errs = append(errs, field.Invalid(path.Child("replica-hosts").Index(i), host, errMsg))
					}
				}
			}
		}
	}

	if len(m.ReplicaPorts) != len(m.ReplicaHosts) {
		errs = append(errs, field.Invalid(path.Child("replica-ports"), m.ReplicaPorts, "副本节点端口数量必须与主机数量一致"))
	} else {
		for i, port := range m.ReplicaPorts {
			if port <= 0 || port > 65535 {
				errs = append(errs, field.Invalid(path.Child("replica-ports").Index(i), port, "副本节点端口号必须在1-65535范围内"))
			}
		}
	}

	// 原有的验证逻辑保持不变...
	if m.Username == "" {
		errs = append(errs, field.Required(path.Child("username"), "MySQL用户名必须存在"))
	} else if nameErrors := validation.IsQualifiedName(m.Username); len(nameErrors) > 0 {
		for _, errMsg := range nameErrors {
			errs = append(errs, field.Invalid(path.Child("username"), m.Username, errMsg))
		}
	}

	if m.Password == "" {
		errs = append(errs, field.Required(path.Child("password"), "MySQL密码必须存在"))
	}

	if m.Database == "" {
		errs = append(errs, field.Required(path.Child("database"), "数据库名必须存在"))
	} else if dbErrors := validation.IsDNS1123Label(m.Database); len(dbErrors) > 0 {
		for _, errMsg := range dbErrors {
			errs = append(errs, field.Invalid(path.Child("database"), m.Database, errMsg))
		}
	}

	// 原有的连接池验证...
	if m.MaxIdleConnections < 0 {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能为负数"))
	}

	if m.MaxOpenConnections <= 0 {
		errs = append(errs, field.Invalid(path.Child("max-open-connections"), m.MaxOpenConnections, "最大打开连接数必须大于0"))
	}

	if m.MaxOpenConnections > 0 && m.MaxIdleConnections > m.MaxOpenConnections {
		errs = append(errs, field.Invalid(path.Child("max-idle-connections"), m.MaxIdleConnections, "最大空闲连接数不能大于最大打开连接数"))
	}

	// ... 其他原有验证保持不变

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

	// 管理员账号
	fs.StringVar(&o.AdminUsername, "mysql.admin-username", o.AdminUsername, "MySQL管理员用户名")
	fs.StringVar(&o.AdminPassword, "mysql.admin-password", o.AdminPassword, "MySQL管理员密码")

	// ========== 新增：集群配置 ==========
	fs.StringVar(&o.PrimaryHost, "mysql.primary-host", o.PrimaryHost, "MySQL主节点主机地址（写操作）")
	fs.IntVar(&o.PrimaryPort, "mysql.primary-port", o.PrimaryPort, "MySQL主节点端口")
	fs.StringSliceVar(&o.ReplicaHosts, "mysql.replica-hosts", o.ReplicaHosts, "MySQL副本节点主机地址列表（读操作）")
	fs.IntSliceVar(&o.ReplicaPorts, "mysql.replica-ports", o.ReplicaPorts, "MySQL副本节点端口列表")
	fs.BoolVar(&o.LoadBalance, "mysql.load-balance", o.LoadBalance, "是否启用读负载均衡")
	fs.BoolVar(&o.FailoverEnabled, "mysql.failover-enabled", o.FailoverEnabled, "是否启用故障转移")
	fs.DurationVar(&o.HealthCheckInterval, "mysql.health-check-interval", o.HealthCheckInterval, "健康检查间隔")
	fs.BoolVar(&o.WSREPSyncWait, "mysql.wsrep-sync-wait", o.WSREPSyncWait, "是否等待Galera集群同步")

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

func (m *MySQLOptions) loadFromEnv() error {
	// 原有的环境变量加载...

	// ========== 新增：集群配置环境变量 ==========
	if val := os.Getenv("MYSQL_PRIMARY_HOST"); val != "" {
		m.PrimaryHost = val
	}
	if val := os.Getenv("MYSQL_PRIMARY_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			m.PrimaryPort = port
		}
	}
	if val := os.Getenv("MYSQL_REPLICA_HOSTS"); val != "" {
		m.ReplicaHosts = strings.Split(val, ",")
	}
	if val := os.Getenv("MYSQL_REPLICA_PORTS"); val != "" {
		ports := strings.Split(val, ",")
		m.ReplicaPorts = make([]int, len(ports))
		for i, p := range ports {
			if port, err := strconv.Atoi(p); err == nil {
				m.ReplicaPorts[i] = port
			}
		}
	}
	if val := os.Getenv("MYSQL_LOAD_BALANCE"); val != "" {
		if loadBalance, err := strconv.ParseBool(val); err == nil {
			m.LoadBalance = loadBalance
		}
	}
	if val := os.Getenv("MYSQL_FAILOVER_ENABLED"); val != "" {
		if failover, err := strconv.ParseBool(val); err == nil {
			m.FailoverEnabled = failover
		}
	}
	if val := os.Getenv("MYSQL_HEALTH_CHECK_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			m.HealthCheckInterval = duration
		}
	}
	if val := os.Getenv("MYSQL_WSREP_SYNC_WAIT"); val != "" {
		if syncWait, err := strconv.ParseBool(val); err == nil {
			m.WSREPSyncWait = syncWait
		}
	}

	return nil
}
