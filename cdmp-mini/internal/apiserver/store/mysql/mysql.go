/*
// 定义datastore 内嵌gorom.DB指针
// *gorm.DB 是数据库连接的核心对象，封装了数据库连接、查询构建、事务等功能。
// datastore 结构体：作为数据库操作的 “容器”，通过持有 *gorm.DB 实例，后续可以为该结构体定义各种数据库操作方法（如增删改查），避免直接在业务逻辑中裸用 *gorm.DB，提高代码的可维护性。
// 封装性：将 *gorm.DB 隐藏在 datastore 内部，业务逻辑只需调用 datastore 的方法，无需直接操作 *gorm.DB，降低耦合度。
// 可测试性：在单元测试中，可以通过替换 datastore 中的 db 为 mock 实例（如 gorm.io/gorm/mock），方便模拟数据库行为。
// 扩展性：若后续需要切换数据库（如从 MySQL 到 PostgreSQL），只需修改 datastore 内部的 db 初始化逻辑，业务层方法无需改动。
// 职责单一：datastore 专注于数据库操作，业务逻辑层专注于业务处理，符合 “单一职责原则”。
// 相当于仓库总调度,持有数据库连接核心资源 可理解为仓库的总钥匙 负责协调各类资源的访问入口
// 通过Users()指派具体的资源管理员(users，，自己不直接处理具体资源的存取 而是作为统一入口分发任务.
// 定义工厂实例//单例模式.mysqlFactory
// 定义sync.Once,于保证某个操作 仅执行一次，无论有多少个 goroutine 同时调用。它常被用来实现单例模式、初始化资源等场景，确保初始化代码只执行一次，避免并发环境下的资源竞争问题。
*/
package mysql

import (
	"fmt"
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/logger"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

var (
	mysqlFactory store.Factory
	once         sync.Once
)

type datastore struct {
	db *gorm.DB
}

func (ds *datastore) Users() store.UserStore {
	return newUsers(ds)
}

func (ds *datastore) Secrets() store.SecretStore {
	return newSecrets(ds)
}

func (ds *datastore) Polices() store.PolicyStore {
	return newPolices(ds)
}

func (ds *datastore) PolicyAudits() store.PolicyAuditStore {
	return newPolicyAudits(ds)
}

func GetMySQLFactoryOr(opts *options.MySQLOptions) (store.Factory, error) {
	if opts == nil && mysqlFactory == nil {
		return nil, fmt.Errorf("获取mysql store factory失败")
	}
	var err error
	var dbIns *gorm.DB

	once.Do(func() {
		options := &db.Options{
			Host:                  opts.Host,
			Username:              opts.Username,
			Password:              opts.Password,
			Database:              opts.Database,
			MaxIdleConnections:    opts.MaxIdleConnections,
			MaxOpenConnections:    opts.MaxOpenConnections,
			MaxConnectionLifeTime: opts.MaxConnectionLifeTime,
			LogLevel:              opts.LogLevel,
			Logger:                logger.New(opts.LogLevel),
		}
		dbIns, err = db.New(options)

		mysqlFactory = &datastore{dbIns}
	})
	if mysqlFactory == nil || err != nil {
		return nil, fmt.Errorf("failed to get mysql store fatory, mysqlFactory: %+v, error: %w", mysqlFactory, err)
	}
	return mysqlFactory, nil
}

func (ds *datastore) Close() error {
	db, err := ds.db.DB()
	if err != nil {
		return errors.Wrap(err, "get gorm db instance failed")
	}

	return db.Close()
}

// cleanDatabase tear downs the database tables.
// nolint:unused // may be reused in the feature, or just show a migrate usage.
func cleanDatabase(db *gorm.DB) error {
	if err := db.Migrator().DropTable(&v1.User{}); err != nil {
		return errors.Wrap(err, "drop user table failed")
	}
	if err := db.Migrator().DropTable(&v1.Policy{}); err != nil {
		return errors.Wrap(err, "drop policy table failed")
	}
	if err := db.Migrator().DropTable(&v1.Secret{}); err != nil {
		return errors.Wrap(err, "drop secret table failed")
	}

	return nil
}

// migrateDatabase run auto migration for given models, will only add missing fields,
// won't delete/change current data.
// nolint:unused // may be reused in the feature, or just show a migrate usage.
func migrateDatabase(db *gorm.DB) error {
	if err := db.AutoMigrate(&v1.User{}); err != nil {
		return errors.Wrap(err, "migrate user model failed")
	}
	if err := db.AutoMigrate(&v1.Policy{}); err != nil {
		return errors.Wrap(err, "migrate policy model failed")
	}
	if err := db.AutoMigrate(&v1.Secret{}); err != nil {
		return errors.Wrap(err, "migrate secret model failed")
	}

	return nil
}

// resetDatabase resets the database tables.
// nolint:unused,deadcode // may be reused in the feature, or just show a migrate usage.
func resetDatabase(db *gorm.DB) error {
	if err := cleanDatabase(db); err != nil {
		return err
	}
	if err := migrateDatabase(db); err != nil {
		return err
	}

	return nil
}
