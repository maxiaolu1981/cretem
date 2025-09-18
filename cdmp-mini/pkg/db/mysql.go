package db

import (
	"context"
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
	// 设置默认参数，防止配置缺失
	setDefaultOptions(opts)
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
	// 关键：添加全局SQL查询超时拦截器（修复回调注册语法）
	addQueryTimeoutCallbacks(db, opts.Timeout)

	log.Infof("Database connection pool initialized: "+
		"MaxOpenConns=%d, MaxIdleConns=%d, ConnMaxLifetime=%v",
		opts.MaxOpenConnections, opts.MaxIdleConnections, opts.MaxConnectionLifeTime)

	return db, nil
}

// 添加全局SQL查询超时回调（修复后的正确写法）
func addQueryTimeoutCallbacks(db *gorm.DB, timeout time.Duration) {
	// 为Create操作添加超时
	db.Callback().Create().Before("gorm:create").Register("query_timeout:create", func(db *gorm.DB) {
		setQueryTimeout(db, timeout)
	})
	db.Callback().Create().After("gorm:create").Register("query_timeout:create_cleanup", cleanupTimeout)

	// 为Query操作添加超时
	db.Callback().Query().Before("gorm:query").Register("query_timeout:query", func(db *gorm.DB) {
		setQueryTimeout(db, timeout)
	})
	db.Callback().Query().After("gorm:query").Register("query_timeout:query_cleanup", cleanupTimeout)

	// 为Update操作添加超时
	db.Callback().Update().Before("gorm:update").Register("query_timeout:update", func(db *gorm.DB) {
		setQueryTimeout(db, timeout)
	})
	db.Callback().Update().After("gorm:update").Register("query_timeout:update_cleanup", cleanupTimeout)

	// 为Delete操作添加超时
	db.Callback().Delete().Before("gorm:delete").Register("query_timeout:delete", func(db *gorm.DB) {
		setQueryTimeout(db, timeout)
	})
	db.Callback().Delete().After("gorm:delete").Register("query_timeout:delete_cleanup", cleanupTimeout)
}

// 为当前SQL设置超时Context
func setQueryTimeout(db *gorm.DB, timeout time.Duration) {
	if db.Statement.Context == nil || db.Statement.Context.Done() == nil {
		// 创建带超时的Context，并存储cancel函数到db实例中（后续清理）
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		db.Statement.Context = ctx
		db.InstanceSet("query_timeout_cancel", cancel)
	}
}

// 清理超时Context的cancel函数，避免资源泄漏
func cleanupTimeout(db *gorm.DB) {
	if cancel, ok := db.InstanceGet("query_timeout_cancel"); ok {
		if c, ok := cancel.(context.CancelFunc); ok {
			c() // 执行cancel，释放资源
		}
		db.InstanceSet("query_timeout_cancel", nil)
	}
}

// 为未配置的参数设置合理默认值
func setDefaultOptions(opts *Options) {
	if opts.MaxOpenConnections <= 0 {
		opts.MaxOpenConnections = 200 // 默认支持200并发连接
	}
	if opts.MaxIdleConnections <= 0 {
		opts.MaxIdleConnections = 50 // 保留50个空闲连接
	}
	if opts.MaxConnectionLifeTime <= 0 {
		opts.MaxConnectionLifeTime = 1 * time.Hour // 连接1小时后重建
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second // 默认10秒连接超时
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 3 * time.Second // 默认3秒查询超时
	}
	// 日志默认值：如果未传Logger，使用内置日志并按LogLevel配置
	if opts.Logger == nil {
		logLevel := logger.Info
		switch opts.LogLevel {
		case 0:
			logLevel = logger.Silent
		case 1:
			logLevel = logger.Error
		case 2:
			logLevel = logger.Warn
		case 3:
			logLevel = logger.Info
		}
		opts.Logger = logger.Default.LogMode(logLevel)
	}
}
