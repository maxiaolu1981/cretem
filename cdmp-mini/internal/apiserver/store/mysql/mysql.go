package mysql

import (
	"errors"
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/db"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/logger"
	"gorm.io/gorm"
)

// 定义工厂实例//单例模式.mysqlFactory
// 定义sync.Once,于保证某个操作 仅执行一次，无论有多少个 goroutine 同时调用。它常被用来实现单例模式、初始化资源等场景，确保初始化代码只执行一次，避免并发环境下的资源竞争问题。
var (
	mysqlFactory store.Factory
	once         sync.Once
)

// 定义datastore 内嵌gorom.DB指针
// *gorm.DB 是数据库连接的核心对象，封装了数据库连接、查询构建、事务等功能。
// datastore 结构体：作为数据库操作的 “容器”，通过持有 *gorm.DB 实例，后续可以为该结构体定义各种数据库操作方法（如增删改查），避免直接在业务逻辑中裸用 *gorm.DB，提高代码的可维护性。
// 封装性：将 *gorm.DB 隐藏在 datastore 内部，业务逻辑只需调用 datastore 的方法，无需直接操作 *gorm.DB，降低耦合度。
// 可测试性：在单元测试中，可以通过替换 datastore 中的 db 为 mock 实例（如 gorm.io/gorm/mock），方便模拟数据库行为。
// 扩展性：若后续需要切换数据库（如从 MySQL 到 PostgreSQL），只需修改 datastore 内部的 db 初始化逻辑，业务层方法无需改动。
// 职责单一：datastore 专注于数据库操作，业务逻辑层专注于业务处理，符合 “单一职责原则”。
// 相当于仓库总调度,持有数据库连接核心资源 可理解为仓库的总钥匙 负责协调各类资源的访问入口
// 通过Users()指派具体的资源管理员(users，，自己不直接处理具体资源的存取 而是作为统一入口分发任务.
type datastore struct {
	db *gorm.DB
}

func (db *datastore) Users() store.UserStore {
	log.Info("mysql:仓库总调度说:好的，我知道了，我通知用户资源管理员")
	return newUsers(db)
}

// 这个 GetMySQLFactoryOr 函数是MySQL 数据库连接工厂的单例创建函数，用于确保全局只初始化一次数据库连接池，并返回统一的数据操作入口（store.Factory）。其核心设计围绕 “单例模式” 和 “资源复用”，避免重复创建数据库连接导致的资源浪费。
// 根据 MySQL 配置选项（opts）创建或获取已有的数据库工厂实例（单例，，返回一个store factory
func GetMySQLFactoryOr(opts *options.MySQLOptions) (store.Factory, error) {
	// 校验：若配置为空且全局工厂未初始化，则返回错误
	if opts == nil && mysqlFactory == nil {
		return nil, errors.New("mysql工厂获取错误.")
	}
	//定义错误
	var err error
	// GORM 数据库连接实例
	var dbIns *gorm.DB
	// 单例初始化：使用 sync.Once 确保以下逻辑仅执行一次
	once.Do(func() {
		//定义数据库全局参数db.Options
		dbOptions := &db.Options{
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
		//实例化db.New 创建 GORM 连接实例（初始化数据库连接池）
		dbIns, err = db.New(dbOptions)

		//初始化全局数据库工厂（datastore 实现了 store.Factory 接口）
		mysqlFactory = &datastore{
			db: dbIns,
		}
	})

	// 校验初始化结果：若工厂未创建或存在错误，返回失败
	if mysqlFactory == nil || err != nil {
		return nil, errors.New("factory error")
	}
	// 返回单例的数据库工厂实例
	return mysqlFactory, nil
}
