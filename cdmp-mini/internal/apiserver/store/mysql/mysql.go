package mysql

import (
	"fmt"
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/db"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/logger"
	"gorm.io/gorm"
)

var (
	mysqlFactory store.Factory
	once         sync.Once
)

type datastore struct {
	db *gorm.DB
}

func (db *datastore) Users() store.UserStore {
	return newUsers(ds)
}

func GetMySQLFactoryOr(opts *options.MySQLOptions) (store.Factory, error) {
	if opts == nil && mysqlFactory == nil {
		return nil, fmt.Errorf("创建mysql工厂失败")
	}
	var err error
	var dbIns *gorm.DB

	once.Do(func() {
		//创建db opetions
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
		//创建gorm链接实例
		dbIns, err = db.New(options)
		mysqlFactory = &datastore{dbIns}
	})

}
