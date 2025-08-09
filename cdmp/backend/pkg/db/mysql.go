// Package db 专注于实现 MySQL 数据库连接的底层创建逻辑，
// 基于传入的配置参数，封装 GORM 框架的初始化过程，
// 为上层业务提供可直接用于数据库操作的 *gorm.DB 实例，
// 是应用与 MySQL 数据库建立连接、配置连接池的核心模块。
//
// 设计意图：
//   - 屏蔽 GORM 初始化的复杂细节（如 DSN 拼接、连接池参数设置等），
//     向上层暴露简洁、清晰的数据库连接创建接口，降低业务代码与 GORM 框架的耦合度。
//   - 集中管理数据库连接相关的配置应用和错误处理，让数据库连接逻辑内聚，
//     便于后续对连接池优化、驱动更换（如从 MySQL 换为其他数据库）等需求进行扩展和维护。
//
// 核心功能：
//  1. 定义 Options 结构体，承载创建数据库连接所需的关键配置参数，
//     涵盖数据库地址、认证信息、连接池参数以及 GORM 日志器等内容。
//  2. 提供 New 方法，按照传入的 Options 配置，完成 GORM 框架的初始化，
//     包括构建数据库连接字符串（DSN）、设置 GORM 配置、初始化连接池参数等操作，
//     最终返回可用于数据库操作的 *gorm.DB 实例，同时处理连接过程中可能出现的错误。
//
// 协作关系：
//   - 依赖 gorm.io/gorm 及其相关驱动（如 gorm.io/driver/mysql）实现数据库连接和操作，
//     遵循 GORM 框架的使用规范进行初始化和配置。
//   - 与上层配置管理模块（如 options 包）协同工作，
//     接收由上层转换后的配置参数（Options 结构体），将配置落地为实际的数据库连接。
//
// 使用示例：
//
//	通常与配置管理模块配合使用，示例流程如下：
//	1. 上层（如 options 包）构造好 db.Options 实例，填充各项配置；
//	2. 调用 db.New(opts) 获取 *gorm.DB 实例；
//	3. 使用该实例进行数据库操作，如 db.Create(&model)、db.First(&result, condition) 等。
//
// 注意事项：
//   - 传入的 Options 配置需保证合法性（如数据库地址可访问、认证信息正确等），
//     否则 New 方法会返回错误，上层需做好错误处理。
//   - 连接池参数（MaxOpenConnections、MaxIdleConns 等）的设置需结合实际业务场景，
//     不合理的参数可能导致数据库性能问题（如连接池过大导致数据库负载过高，
//     或过小导致连接竞争激烈影响吞吐量）。
//
// 初始化流程 → MySQLOptions（业务配置层） → db.Options（数据库连接层） → GORM 初始化
package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Options 定义了 MySQL 数据库的配置选项
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

// New 根据给定的配置选项创建一个新的 gorm DB 实例
func New(opts *Options) (*gorm.DB, error) {
	// 构建 MySQL 连接字符串（DSN）
	dsn := fmt.Sprintf(
		`%s:%s@tcp(%s)/%s?charset=utf8&parseTime=%t&loc=%s`,
		opts.Username, // 用户名
		opts.Password, // 密码
		opts.Host,     // 主机地址
		opts.Database, // 数据库名
		true,          // 启用 parseTime 选项（解析时间字段）
		"Local",       // 设置时区为本地时区
	)

	// 使用 GORM 打开 MySQL 连接，传入自定义日志器
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: opts.Logger, // 使用配置中的日志器
	})
	if err != nil {
		return nil, err // 连接失败时返回错误
	}

	// 获取底层的 SQL 数据库对象（*sql.DB），用于设置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 设置与数据库的最大打开连接数
	sqlDB.SetMaxOpenConns(opts.MaxOpenConnections)

	// 设置连接的最大可重用时间（超过此时长后连接将被关闭）
	sqlDB.SetConnMaxLifetime(opts.MaxConnectionLifeTime)

	// 设置连接池中的最大空闲连接数
	sqlDB.SetMaxIdleConns(opts.MaxIdleConnections)

	// 返回初始化完成的 GORM DB 实例
	return db, nil
}
