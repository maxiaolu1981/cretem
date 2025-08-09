// 版权所有 2020 Lingfei Kong <colin404@foxmail.com>。保留所有权利。
// 本源代码的使用受 MIT 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package options

import (
	"time"

	"github.com/spf13/pflag"
	"gorm.io/gorm"

	"github.com/marmotedu/iam/pkg/db"
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

	return errs // 目前未实现具体验证逻辑，返回空错误列表
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

// NewClient 根据给定的配置创建 MySQL 数据库客户端
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
