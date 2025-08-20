package options

import (
	"fmt"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation"
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

func (m *MySQLOptions) AddFlags(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.StringVar(&m.Host, "mysql.host", m.Host, "MySQL 服务主机地址。若留空，后续相关的 MySQL 选项将被忽略。")

	fs.StringVar(&m.Username, "mysql.username", m.Username, "访问 MySQL 服务的用户名。")

	fs.StringVar(&m.Password, "mysql.password", m.Password, "访问 MySQL 的密码，应与用户名配套使。。")

	fs.StringVar(&m.Database, "mysql.database", m.Database, "服务器要使用的数据库名称。")

	fs.IntVar(&m.MaxIdleConnections, "mysql.max-idle-connections", m.MaxIdleConnections, "允许连接到 MySQL 的最大空闲连接数。")

	fs.IntVar(&m.MaxOpenConnections, "mysql.max-open-connections", m.MaxOpenConnections, "允许连接到Mysql的最大连接数")

	fs.DurationVar(&m.MaxConnectionLifeTime, "mysql.max-connection-lifetime", m.MaxConnectionLifeTime, "连接最大生命周期")

	fs.IntVar(&m.LogLevel, "mysql.loglevel", m.LogLevel, "指定gorm日志级别,")
}

func (m *MySQLOptions) Validate() []error {
	errs := []error{}

	host := strings.TrimSpace(m.Host)
	username := strings.TrimSpace(m.Username)
	database := strings.TrimSpace(m.Database)

	if host == "" {
		return errs
	}
	if username == "" && m.Username != "" {
		errs = append(errs, fmt.Errorf("用户名不能为纯空格"))
	}
	if err := validation.IsValidPassword(m.Password); err != nil {
		errs = append(errs, err)
	}

	if database == "" {
		errs = append(errs, fmt.Errorf("%s:数据库名称不能为空", m.Database))
	}

	if m.MaxOpenConnections <= 0 {
		errs = append(errs, fmt.Errorf("最大打开连接数不能小于等于零,当前值:%d", m.MaxOpenConnections))
	}
	if m.MaxIdleConnections < 0 {
		errs = append(errs, fmt.Errorf("空闲连接数不能小于零,当前值:%d", m.MaxIdleConnections))
	}
	if m.MaxIdleConnections > m.MaxOpenConnections {
		errs = append(errs, fmt.Errorf("最大空闲连接数(%d)不能大于最大连接数(%d)", m.MaxIdleConnections, m.MaxOpenConnections))
	}
	if m.MaxConnectionLifeTime < 0 {
		errs = append(errs, fmt.Errorf("最大生命链接周期不能小于0,当前值%d", m.MaxConnectionLifeTime))
	}
	if m.LogLevel < 0 || m.LogLevel > 4 {
		errs = append(errs, fmt.Errorf("gorm日志级别必须在0-4之间,当前值%d", m.LogLevel))
	}
	return errs
}
