/*
mysql 包实现了基于 MySQL 数据库的存储层工厂（store.Factory），提供了与用户（User）、密钥（Secret）、策略（Policy）等数据模型相关的数据库操作接口。核心功能包括：
基于 GORM 框架初始化 MySQL 连接；
提供统一的数据库操作入口（如 Users()、Secrets() 等方法）；
支持数据库连接的关闭、表结构迁移（migrateDatabase）和清理（cleanDatabase）；
通过单例模式（sync.Once）确保数据库连接唯一，避免重复初始化。
核心流程
初始化数据库连接
通过 GetMySQLFactoryOr 函数，基于 MySQL 配置（genericoptions.MySQLOptions）创建数据库连接：
使用 sync.Once 保证连接只初始化一次（单例模式）；
将配置转换为数据库连接选项（db.Options），通过 db.New 建立 GORM 连接；
可选：调用 migrateDatabase 自动迁移表结构（创建或更新与数据模型对应的表）。
提供数据操作接口
初始化后的 datastore 实例通过方法（Users()、Secrets() 等）返回具体数据模型的操作对象（如 users、secrets），这些对象封装了对对应表的 CRUD 操作。
关闭数据库连接
调用 Close() 方法关闭数据库连接，释放资源（通过 GORM 底层的数据库连接池实现）。
*/
package mysql

import (
	"fmt"
	"sync"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1" // 自定义库：API 数据模型（用户、策略等）
	"github.com/maxiaolu1981/cretem/nexuscore/errors"              // 自定义库：错误处理工具
	"gorm.io/gorm"                                                 // 第三方库：GORM ORM 框架

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"            // 自定义库：存储层接口定义
	genericoptions "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options" // 自定义库：通用配置选项
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db"                              // 自定义库：数据库连接工具
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/logger"                          // 自定义库：日志工具
)

// datastore 实现了 store.Factory 接口，封装了 MySQL 数据库连接及数据操作
type datastore struct {
	db *gorm.DB // GORM 数据库连接实例

	// 如需多数据库实例，可在此扩展（如：docker *gorm.DB）
}

// Users 返回用户数据操作接口（UserStore）
func (ds *datastore) Users() store.UserStore {
	return newUsers(ds) // newUsers 是用户操作的具体实现（未在此展示）
}

// Secrets 返回密钥数据操作接口（SecretStore）
func (ds *datastore) Secrets() store.SecretStore {
	return newSecrets(ds) // newSecrets 是密钥操作的具体实现（未在此展示）
}

// Policies 返回策略数据操作接口（PolicyStore）
func (ds *datastore) Policies() store.PolicyStore {
	return newPolicies(ds) // newPolicies 是策略操作的具体实现（未在此展示）
}

// PolicyAudits 返回策略审计数据操作接口（PolicyAuditStore）
func (ds *datastore) PolicyAudits() store.PolicyAuditStore {
	return newPolicyAudits(ds) // newPolicyAudits 是策略审计操作的具体实现（未在此展示）
}

// Close 关闭数据库连接，释放资源
func (ds *datastore) Close() error {
	// 获取 GORM 底层的数据库连接（*sql.DB）
	db, err := ds.db.DB()
	if err != nil {
		return errors.Wrap(err, "获取 GORM 底层数据库实例失败")
	}

	// 关闭数据库连接
	return db.Close()
}

var (
	mysqlFactory store.Factory // MySQL 存储工厂单例
	once         sync.Once     // 用于确保初始化只执行一次（单例模式）
)

// GetMySQLFactoryOr 基于配置创建或获取已有的 MySQL 存储工厂（单例）
func GetMySQLFactoryOr(opts *genericoptions.MySQLOptions) (store.Factory, error) {
	// 若配置为空且工厂未初始化，则返回错误
	if opts == nil && mysqlFactory == nil {
		return nil, fmt.Errorf("获取 MySQL 存储工厂失败")
	}

	var err error
	var dbIns *gorm.DB // 数据库连接实例

	// 确保初始化逻辑只执行一次（线程安全）
	once.Do(func() {
		// 构建数据库连接选项（从配置转换）
		options := &db.Options{
			Host:                  opts.Host,                  // 数据库主机地址
			Username:              opts.Username,              // 用户名
			Password:              opts.Password,              // 密码
			Database:              opts.Database,              // 数据库名
			MaxIdleConnections:    opts.MaxIdleConnections,    // 最大空闲连接数
			MaxOpenConnections:    opts.MaxOpenConnections,    // 最大打开连接数
			MaxConnectionLifeTime: opts.MaxConnectionLifeTime, // 连接最大存活时间
			LogLevel:              opts.LogLevel,              // 日志级别
			Logger:                logger.New(opts.LogLevel),  // 日志实例
		}

		// 创建 GORM 数据库连接
		dbIns, err = db.New(options)

		// 取消注释以下行可自动迁移表结构（生产环境不建议启用）
		// migrateDatabase(dbIns)

		// 初始化存储工厂实例
		mysqlFactory = &datastore{dbIns}
	})

	// 检查工厂是否初始化成功
	if mysqlFactory == nil || err != nil {
		return nil, fmt.Errorf("获取 MySQL 存储工厂失败，mysqlFactory: %+v, 错误: %w", mysqlFactory, err)
	}

	return mysqlFactory, nil
}

// cleanDatabase 删除数据库中所有表（用于测试或重置）
// nolint:unused // 可能在未来复用，或作为示例展示
func cleanDatabase(db *gorm.DB) error {
	if err := db.Migrator().DropTable(&v1.User{}); err != nil {
		return errors.Wrap(err, "删除用户表失败")
	}
	if err := db.Migrator().DropTable(&v1.Policy{}); err != nil {
		return errors.Wrap(err, "删除策略表失败")
	}
	if err := db.Migrator().DropTable(&v1.Secret{}); err != nil {
		return errors.Wrap(err, "删除密钥表失败")
	}

	return nil
}

// migrateDatabase 自动迁移表结构（仅添加缺失字段，不删除现有数据）
// nolint:unused // 可能在未来复用，或作为示例展示
func migrateDatabase(db *gorm.DB) error {
	// 迁移用户表（根据 v1.User 模型创建或更新表）
	if err := db.AutoMigrate(&v1.User{}); err != nil {
		return errors.Wrap(err, "迁移用户模型失败")
	}
	// 迁移策略表
	if err := db.AutoMigrate(&v1.Policy{}); err != nil {
		return errors.Wrap(err, "迁移策略模型失败")
	}
	// 迁移密钥表
	if err := db.AutoMigrate(&v1.Secret{}); err != nil {
		return errors.Wrap(err, "迁移密钥模型失败")
	}

	return nil
}

// resetDatabase 重置数据库表（先删除再迁移，清空数据）
// nolint:unused,deadcode // 可能在未来复用，或作为示例展示
func resetDatabase(db *gorm.DB) error {
	if err := cleanDatabase(db); err != nil {
		return err
	}
	if err := migrateDatabase(db); err != nil {
		return err
	}

	return nil
}
