package db

import (
	"fmt"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
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
	TablePrefix           string           // 新增：表前缀
	Timeout               time.Duration    // 新增：连接超时
}

func New(opts *Options) (*gorm.DB, error) {
	// 定义dsn - 添加更多优化参数
	dsn := fmt.Sprintf(`%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=%t&loc=%s&timeout=10s&readTimeout=30s&writeTimeout=30s`,
		opts.Username,
		opts.Password,
		opts.Host,
		opts.Database,
		true,
		"Local")

	// gorm.Open()打开gorm链接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		PrepareStmt:                              true, // 预编译语句，提高性能
		SkipDefaultTransaction:                   true, // 禁用默认事务，提高性能
		DisableForeignKeyConstraintWhenMigrating: true, // 迁移时禁用外键约束
		// 添加NamingStrategy确保表名一致性
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   opts.TablePrefix, // 可选：表前缀
			SingularTable: true,             // 使用单数表名
		},
	})

	if err != nil {
		log.Errorf("failed to open database: %v", err)
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// 获取底层sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		log.Errorf("failed to get sql.DB: %v", err)
		return nil, fmt.Errorf("failed to get sql.DB: %v", err)
	}

	// 设置连接池配置
	sqlDB.SetMaxOpenConns(opts.MaxOpenConnections)       // 最大打开连接数
	sqlDB.SetConnMaxLifetime(opts.MaxConnectionLifeTime) // 连接最大生命周期
	sqlDB.SetMaxIdleConns(opts.MaxIdleConnections)       // 最大空闲连接数
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)           // 空闲连接最大存活时间

	// 添加连接健康检查
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed: %v", err)
	}

	log.Infof("Database connection pool initialized: "+
		"MaxOpenConns=%d, MaxIdleConns=%d, ConnMaxLifetime=%v",
		opts.MaxOpenConnections, opts.MaxIdleConnections, opts.MaxConnectionLifeTime)

	return db, nil
}
