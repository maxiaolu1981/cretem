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
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

var (
	mysqlFactory interfaces.Factory
	dbManager    *db.DBManager
	once         sync.Once
)

type Datastore struct {
	DB         *gorm.DB      // 主数据库连接（写操作）
	DBManager  *db.DBManager // 数据库管理器（集群模式）
	UseCluster bool          // 是否使用集群模式
}

// 新增：创建函数
func newUsers(ds *Datastore) interfaces.UserStore {
	policyStore := newPolices(ds)
	if ds.UseCluster {
		// 集群模式下，读操作使用负载均衡
		return user.NewUsers(ds.DBManager.GetReadDB(), policyStore)
	}
	return user.NewUsers(ds.DB, policyStore)
}

func newPolices(ds *Datastore) interfaces.PolicyStore {
	if ds.UseCluster {
		return &policy.Policy{Db: ds.DBManager.GetReadDB()}
	}
	return &policy.Policy{Db: ds.DB}
}

func newSecrets(ds *Datastore) interfaces.SecretStore {
	if ds.UseCluster {
		return &secret.Secret{Db: ds.DBManager.GetReadDB()}
	}
	return &secret.Secret{Db: ds.DB}
}

func newPolicyAudit(ds *Datastore) interfaces.PolicyAuditStore {
	if ds.UseCluster {
		return &policyaudit.Policy_audit{Db: ds.DBManager.GetReadDB()}
	}
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

// 写操作使用主数据库
func (ds *Datastore) WriteDB() *gorm.DB {
	if ds.UseCluster {
		return ds.DBManager.GetWriteDB()
	}
	return ds.DB
}

// 读操作使用负载均衡（集群模式）或主数据库（单机模式）
func (ds *Datastore) ReadDB() *gorm.DB {
	if ds.UseCluster {
		return ds.DBManager.GetReadDB()
	}
	return ds.DB
}

// 获取集群状态
func (ds *Datastore) ClusterStatus() db.ClusterStatus {
	if ds.UseCluster && ds.DBManager != nil {
		return ds.DBManager.GetClusterStatus()
	}
	return db.ClusterStatus{
		PrimaryHealthy:  true,
		ReplicaCount:    1,
		HealthyReplicas: 1,
		LoadBalance:     false,
		FailoverEnabled: false,
	}
}

// 判断是否使用集群模式
func shouldUseCluster(opts *options.MySQLOptions) bool {
	// 如果配置了副本节点且启用了负载均衡，则使用集群模式
	return opts != nil &&
		len(opts.ReplicaHosts) > 0 &&
		opts.LoadBalance &&
		len(opts.ReplicaHosts) == len(opts.ReplicaPorts)
}

func GetMySQLFactoryOr(opts *options.MySQLOptions) (interfaces.Factory, *gorm.DB, error) {
	if opts == nil && mysqlFactory == nil {
		return nil, nil, fmt.Errorf("获取mysql store factory失败")
	}
	var err error
	var dbIns *gorm.DB

	once.Do(func() {
		useCluster := shouldUseCluster(opts)

		if useCluster {
			// 集群模式
			dbOptions := &db.Options{
				// 基础配置
				Host:                  opts.Host,
				Username:              opts.Username,
				Password:              opts.Password,
				Database:              opts.Database,
				MaxIdleConnections:    opts.MaxIdleConnections,
				MaxOpenConnections:    opts.MaxOpenConnections,
				MaxConnectionLifeTime: opts.MaxConnectionLifeTime,
				LogLevel:              opts.LogLevel,
				Logger:                logger.New(opts.LogLevel),
				TablePrefix:           "",
				Timeout:               opts.DialTimeout,

				// 集群配置
				PrimaryHost:         opts.PrimaryHost,
				PrimaryPort:         opts.PrimaryPort,
				ReplicaHosts:        opts.ReplicaHosts,
				ReplicaPorts:        opts.ReplicaPorts,
				LoadBalance:         opts.LoadBalance,
				FailoverEnabled:     opts.FailoverEnabled,
				HealthCheckInterval: opts.HealthCheckInterval,
				MaxRetryAttempts:    opts.MaxRetryAttempts,
				RetryInterval:       opts.RetryInterval,
				ReadTimeout:         opts.ReadTimeout,
				WriteTimeout:        opts.WriteTimeout,
				DialTimeout:         opts.DialTimeout,
				ConnMaxLifetime:     opts.ConnMaxLifetime,
				ConnMaxIdleTime:     opts.ConnMaxIdleTime,
			}

			dbManager, err = db.NewDBManager(dbOptions)
			if err != nil {
				return
			}

			// 使用主数据库作为默认连接（向后兼容）
			dbIns = dbManager.GetWriteDB()
			mysqlFactory = &Datastore{
				DB:         dbIns,
				DBManager:  dbManager,
				UseCluster: true,
			}

			log.Infof("MySQL cluster factory initialized with %d replica nodes", len(opts.ReplicaHosts))
		} else {
			// 单机模式（向后兼容）
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
				TablePrefix:           "",
				Timeout:               opts.DialTimeout,

				// 设置单机模式下的集群配置
				PrimaryHost:         opts.Host,
				PrimaryPort:         opts.Port,
				ReplicaHosts:        []string{opts.Host},
				ReplicaPorts:        []int{opts.Port},
				LoadBalance:         false,
				FailoverEnabled:     false,
				HealthCheckInterval: 0,
				MaxRetryAttempts:    opts.MaxRetryAttempts,
				RetryInterval:       opts.RetryInterval,
				ReadTimeout:         opts.ReadTimeout,
				WriteTimeout:        opts.WriteTimeout,
				DialTimeout:         opts.DialTimeout,
				ConnMaxLifetime:     opts.ConnMaxLifetime,
				ConnMaxIdleTime:     opts.ConnMaxIdleTime,
			}

			dbIns, err = db.New(dbOptions)
			if err != nil {
				return
			}

			mysqlFactory = &Datastore{
				DB:         dbIns,
				DBManager:  nil,
				UseCluster: false,
			}

			log.Info("MySQL single node factory initialized")
		}
	})

	if mysqlFactory == nil || err != nil {
		return nil, nil, fmt.Errorf("failed to get mysql store factory, mysqlFactory: %+v, error: %w", mysqlFactory, err)
	}
	return mysqlFactory, dbIns, nil
}

func (ds *Datastore) Close() error {
	if ds.UseCluster && ds.DBManager != nil {
		// 关闭集群管理器
		return ds.DBManager.Close()
	}

	// 单机模式关闭数据库连接
	if ds.DB != nil {
		db, err := ds.DB.DB()
		if err != nil {
			return errors.Wrap(err, "get gorm db instance failed")
		}
		return db.Close()
	}

	return nil
}

// 新增：获取数据库管理器（用于高级操作）
func (ds *Datastore) GetDBManager() *db.DBManager {
	return ds.DBManager
}

// 新增：检查是否使用集群模式
func (ds *Datastore) IsClusterMode() bool {
	return ds.UseCluster
}
