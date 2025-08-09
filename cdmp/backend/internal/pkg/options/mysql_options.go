// Package options 主要用于定义与 MySQL 数据库配置相关的选项结构，
// 负责处理配置的初始化、命令行参数绑定以及基于配置创建数据库客户端等功能，
// 是应用中数据库配置管理的核心部分，为上层业务提供统一的数据库连接配置能力。
//
// 设计意图：
//   - 封装 MySQL 数据库连接所需的各类配置参数，实现配置的集中管理，
//     方便从命令行、配置文件（如结合 mapstructure 做配置解析场景）等不同来源加载配置。
//   - 通过清晰的结构体和方法，分离配置定义、参数校验、命令行绑定、客户端创建等职责，
//     让数据库配置相关逻辑内聚且易于维护和扩展，同时与底层数据库操作包（如 db 包）解耦。
//
// 核心功能：
//  1. 定义 MySQLOptions 结构体，承载 MySQL 数据库连接的各项配置参数。
//  2. 提供 NewMySQLOptions 方法初始化默认配置，作为配置的“零值”基础。
//  3. 实现 Validate 方法（当前为预留，可扩展参数校验逻辑），保障配置合法性。
//  4. AddFlags 方法用于将配置参数绑定到命令行 FlagSet，支持通过命令行参数覆盖默认配置。
//  5. NewClient 方法基于配置创建 GORM 数据库客户端，衔接配置与实际数据库连接操作。
//
// 依赖说明：
//   - 依赖 github.com/spf13/pflag 处理命令行参数绑定。
//   - 依赖 gorm.io/gorm 进行数据库客户端相关操作，与 db 包协同完成数据库连接初始化。
//   - 依赖自定义的 db 包（github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db），
//     该包封装了更底层的数据库连接创建逻辑。
package options

import (
	"fmt"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db"
	"github.com/spf13/pflag"
	"gorm.io/gorm"
)

// MySQLOptions 定义了 MySQL 数据库的配置选项
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

// NewMySQLOptions 创建一个"零值"的 MySQLOptions 实例
func NewMySQLOptions() *MySQLOptions {
	return &MySQLOptions{
		Host:                  "127.0.0.1:3306",                // 默认 MySQL 主机地址
		Username:              "root",                          // 默认用户名为空
		Password:              "iam59!z$",                      // 默认密码为空
		Database:              "iam",                           // 默认数据库名为空
		MaxIdleConnections:    100,                             // 默认最大空闲连接数为 100
		MaxOpenConnections:    100,                             // 默认最大打开连接数为 100
		MaxConnectionLifeTime: time.Duration(10) * time.Second, // 默认连接最大生命周期为 10 秒
		LogLevel:              1,                               // 日志级别默认为 1（静默模式）//Gorm内部日志级别
	}
}

// Validate 验证传递给 MySQLOptions 的参数是否有效
func (o *MySQLOptions) Validate() []error {
	errs := []error{}
	// 校验主机地址（若为空，后续数据库连接必然失败）
	if o.Host == "" {
		errs = append(errs, fmt.Errorf("mysql.host 不能为空，请指定 MySQL 服务的主机地址（如 127.0.0.1:3306）"))
	}

	// 校验数据库名（若为空，无法确定要连接的具体数据库）
	if o.Database == "" {
		errs = append(errs, fmt.Errorf("mysql.database 不能为空，请指定要使用的数据库名称"))
	}

	// 校验最大空闲连接数（不能为负数，0 表示不保留空闲连接）
	if o.MaxIdleConnections < 0 {
		errs = append(errs, fmt.Errorf("mysql.max-idle-connections 不能为负数，当前值: %d", o.MaxIdleConnections))
	}

	// 校验最大打开连接数（不能为负数，0 表示无限制，需谨慎使用）
	if o.MaxOpenConnections < 0 {
		errs = append(errs, fmt.Errorf("mysql.max-open-connections 不能为负数，当前值: %d", o.MaxOpenConnections))
	}

	// 校验连接最大生命周期（必须为正数，否则连接可能永久存活导致失效）
	if o.MaxConnectionLifeTime <= 0 {
		errs = append(errs, fmt.Errorf("mysql.max-connection-life-time 必须大于 0，当前值: %v", o.MaxConnectionLifeTime))
	}

	// 校验日志级别（GORM 日志级别通常为非负数，可根据实际定义范围调整）
	if o.LogLevel < 0 {
		errs = append(errs, fmt.Errorf("mysql.log-mode 不能为负数，当前值: %d", o.LogLevel))
	}

	return errs
}

// AddFlags 为特定 API 服务器添加与 MySQL 存储相关的命令行参数到指定的 FlagSet
func (o *MySQLOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Host, "mysql.host", o.Host, ""+
		"MySQL 服务主机地址。如果留空，以下相关的 MySQL 选项将被忽略。")

	fs.StringVar(&o.Username, "mysql.username", o.Username, ""+
		"访问 MySQL 服务的用户名。")

	fs.StringVar(&o.Password, "mysql.password", o.Password, ""+
		"访问 MySQL 的密码，应与用户名配合使用。")

	fs.StringVar(&o.Database, "mysql.database", o.Database, ""+
		"服务器要使用的数据库名称。")

	fs.IntVar(&o.MaxIdleConnections, "mysql.max-idle-connections", o.MaxOpenConnections, ""+
		"允许连接到 MySQL 的最大空闲连接数。")

	fs.IntVar(&o.MaxOpenConnections, "mysql.max-open-connections", o.MaxOpenConnections, ""+
		"允许连接到 MySQL 的最大打开连接数。")

	fs.DurationVar(&o.MaxConnectionLifeTime, "mysql.max-connection-life-time", o.MaxConnectionLifeTime, ""+
		"允许连接到 MySQL 的最大连接生命周期。")

	fs.IntVar(&o.LogLevel, "mysql.log-mode", o.LogLevel, ""+
		"指定 gorm 的日志级别。")
}

// NewClient 根据给定的配置创建 MySQL 数据库客户端.grom
func (o *MySQLOptions) NewClient() (*gorm.DB, error) {
	opts := &db.Options{
		Host:                  o.Host,
		Username:              o.Username,
		Password:              o.Password,
		Database:              o.Database,
		MaxIdleConnections:    o.MaxIdleConnections,
		MaxOpenConnections:    o.MaxOpenConnections,
		MaxConnectionLifeTime: o.MaxConnectionLifeTime,
		LogLevel:              o.LogLevel,
	}

	return db.New(opts) // 调用 db 包的 New 方法创建数据库连接
}
