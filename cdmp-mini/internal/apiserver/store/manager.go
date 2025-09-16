package store

import (
	"fmt"
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/policy"
	policyaudit "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/policy_audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/secret"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/user"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/logger"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/db"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

var (
	mysqlFactory interfaces.Factory
	once         sync.Once
)

type Datastore struct {
	DB *gorm.DB
}

// 新增：创建函数
func newUsers(ds *Datastore) interfaces.UserStore {
	policyStore := newPolices(ds)
	return user.NewUsers(ds.DB, policyStore)
}

func newPolices(ds *Datastore) interfaces.PolicyStore {
	return &policy.Policy{Db: ds.DB}
}

func newSecrets(ds *Datastore) interfaces.SecretStore {
	return &secret.Secret{Db: ds.DB}
}
func newPolicyAudit(ds *Datastore) interfaces.PolicyAuditStore {
	return &policyaudit.Policy_audit{Db: ds.DB}
}

func (ds *Datastore) Users() interfaces.UserStore {
	return newUsers(ds)
}

func (ds *Datastore) Secrets() interfaces.SecretStore {
	return newSecrets(ds)
}

func (ds *Datastore) Polices() interfaces.PolicyStore {
	return newPolices(ds)
}

func (ds *Datastore) PolicyAudits() interfaces.PolicyAuditStore {
	return newPolicyAudit(ds)
}

func GetMySQLFactoryOr(opts *options.MySQLOptions) (interfaces.Factory, *gorm.DB, error) {
	if opts == nil && mysqlFactory == nil {
		return nil, nil, fmt.Errorf("获取mysql store factory失败")
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

		mysqlFactory = &Datastore{dbIns}
	})
	if mysqlFactory == nil || err != nil {
		return nil, nil, fmt.Errorf("failed to get mysql store fatory, mysqlFactory: %+v, error: %w", mysqlFactory, err)
	}
	return mysqlFactory, dbIns, nil
}

func (ds *Datastore) Close() error {
	db, err := ds.DB.DB()
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
