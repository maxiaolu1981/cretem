package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Options struct {
	Host                  string           // 数据库主机地址（如 "127.0.0.1:3306"）
	Username              string           // 数据库访问用户名
	Password              string           // 数据库访问密码
	Database              string           // 要连接的数据库名
	MaxIdleConnections    int              // 连接池中的最大空闲连接数
	MaxOpenConnections    int              // 与数据库的最大打开连接数
	MaxConnectionLifeTime time.Duration    // 连接的最大可重用时间
	LogLevel              int              // 日志级别（GORM 日志详细程度）
	Logger                logger.Interface // GORM 日志器实例（用于自定义日志输出）
}

func New(opts *Options) (*gorm.DB, error) {
	//定义dsn
	dsn := fmt.Sprintf(`%s:%s@tcp(%s)/%s?charset=utf8&parseTime=%t&loc=%s`,
		opts.Username,
		opts.Password,
		opts.Host,
		opts.Database,
		true,
		"Local")
	//gorm.Open()打开gorm链接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: opts.Logger,
	})
	if err != nil {
		return nil, err
	}
	//db.DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	//设置实例最大MaxOpenConnections
	sqlDB.SetMaxOpenConns(opts.MaxOpenConnections)
	//MaxConnectionLifeTime
	sqlDB.SetConnMaxLifetime(opts.MaxConnectionLifeTime)
	//MaxIdleConnections
	sqlDB.SetMaxIdleConns(opts.MaxIdleConnections)
	return db, nil

}
