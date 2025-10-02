package store

import (
	"context"
	"fmt"
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/policy"
	policyaudit "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/policy_audit"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/secret"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/logger"
	moptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/db"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
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

func newUsers(ds *Datastore) interfaces.UserStore {
	log.Infof("🔍 UserStore创建: 集群模式=%v", ds.UseCluster)
	policyStore := newPolices(ds)
	if ds.UseCluster {
		log.Info("🚀 读写分离已启用 - 使用ClusterAwareUserStore")
		// 集群模式下传入读写两个DB
		return &ClusterAwareUserStore{
			readStore:  user.NewUsers(ds.DBManager.GetReadDB(), policyStore),
			writeStore: user.NewUsers(ds.DBManager.GetWriteDB(), policyStore),
		}
	}
	log.Info("💾 单机模式 - 使用普通Users")
	return user.NewUsers(ds.DB, policyStore)
}

// 新增：智能路由的UserStore包装器
type ClusterAwareUserStore struct {
	readStore  interfaces.UserStore // 用于读操作
	writeStore interfaces.UserStore // 用于写操作
}

func (c *ClusterAwareUserStore) Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	log.Infof("📖 读操作路由到从库: username=%s", username)
	return c.readStore.Get(ctx, username, opts, opt) // 读操作用读库
}

func (c *ClusterAwareUserStore) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {
	log.Infof("✏️ 写操作路由到主库: username=%s", user.Name)
	return c.writeStore.Create(ctx, user, opts, opt) // 写操作用写库
}

func (c *ClusterAwareUserStore) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error {
	log.Infof("✏️ update操作路由到主库: username=%s", user.Name)
	return c.writeStore.Update(ctx, user, opts, opt) // 写操作用写库
}

func (c *ClusterAwareUserStore) Delete(ctx context.Context, username string, opts metav1.DeleteOptions, opt *options.Options) error {
	log.Infof("✏️ delete操作路由到主库: username=%s", username)
	return c.writeStore.Delete(ctx, username, opts, opt) // 写操作用写库
}

func (c *ClusterAwareUserStore) DeleteForce(ctx context.Context, username string, opts metav1.DeleteOptions, opt *options.Options) error {
	log.Infof("✏️ deleteforce操作路由到主库: username=%s", username)
	return c.writeStore.DeleteForce(ctx, username, opts, opt) // 写操作用写库
}

func (c *ClusterAwareUserStore) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions, opt *options.Options) error {
	log.Infof("✏️ deletecollection操作路由到主库: username=%v", usernames)
	return c.writeStore.DeleteCollection(ctx, usernames, opts, opt) // 写操作用写库
}

func (c *ClusterAwareUserStore) List(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error) {
	log.Info("✏️ list操作路由到主库: username=")
	return c.readStore.List(ctx, opts, opt) // 读操作用读库
}

func (c *ClusterAwareUserStore) ListAllUsernames(ctx context.Context) ([]string, error) {
	log.Info("✏️ ListAllUsernames: username=")
	return c.readStore.ListAllUsernames(ctx) // 读操作用读库
}

func (c *ClusterAwareUserStore) ListAll(ctx context.Context, username string) (*v1.UserList, error) {

	return c.readStore.ListAll(ctx, username) // 读操作用读库
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
func shouldUseCluster(opts *moptions.MySQLOptions) bool {
	// 如果配置了副本节点且启用了负载均衡，则使用集群模式
	return opts != nil &&
		len(opts.ReplicaHosts) > 0 &&
		opts.LoadBalance &&
		len(opts.ReplicaHosts) == len(opts.ReplicaPorts)
}

func GetMySQLFactoryOr(opts *moptions.MySQLOptions) (interfaces.Factory, *gorm.DB, error) {
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
