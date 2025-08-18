package options

import (
	"time"
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
