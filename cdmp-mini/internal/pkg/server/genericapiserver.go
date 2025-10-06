package server

import (
	"context"
	"os"
	"sync"

	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/segmentio/kafka-go"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"

	mysql "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/db"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

type GenericAPIServer struct {
	insecureServer *http.Server
	*gin.Engine
	options        *options.Options
	redis          *storage.RedisCluster
	redisCancel    context.CancelFunc
	initOnce       sync.Once
	producer       *UserProducer
	consumerCtx    context.Context
	consumerCancel context.CancelFunc
	//createConsumer *UserConsumer
	//updateConsumer *UserConsumer
	//deleteConsumer *UserConsumer
	//retryConsumer  *RetryConsumer
}

func NewGenericAPIServer(opts *options.Options) (*GenericAPIServer, error) {
	// 初始化日志
	log.Debugf("正在初始化GenericAPIServer服务器，环境: %s", opts.ServerRunOptions.Mode)
	// 打印 Kafka 实例ID
	if opts.KafkaOptions != nil {
		log.Infof("[Kafka] 当前实例 InstanceID = %s", opts.KafkaOptions.InstanceID)
	}

	//创建服务器实例
	g := &GenericAPIServer{
		Engine:   gin.New(),
		options:  opts,
		initOnce: sync.Once{},
	}

	//设置gin运行模式
	if err := g.configureGin(); err != nil {
		return nil, err
	}
	// 初始化mysql
	storeIns, dbIns, err := mysql.GetMySQLFactoryOr(opts.MysqlOptions)
	if err != nil {
		log.Error("mysql服务器启动失败")
		return nil, err
	}
	interfaces.SetClient(storeIns)
	log.Debug("mysql服务器初始化成功")

	// ========== 新增：增强版集群状态检查和初始化 ==========
	if datastore, ok := storeIns.(*mysql.Datastore); ok {
		if datastore.IsClusterMode() {
			log.Debug("🚀 检测到Galera集群模式，正在初始化集群连接...")

			// 执行集群健康检查
			if err := initializeGaleraCluster(datastore); err != nil {
				log.Errorf("Galera集群初始化警告: %v", err)
				// 不阻止启动，但记录警告
			}

			// 定期监控集群状态（可选）
			go monitorClusterHealth(datastore, opts.MysqlOptions.HealthCheckInterval)
		} else {
			log.Debug("✅ 使用单节点MySQL模式")
		}
	}

	time.Sleep(10 * time.Second)

	//初始化redis
	if err := g.initRedisStore(); err != nil {
		log.Error("redis服务器启动失败")
		return nil, err
	}
	log.Debug("redis服务器启动成功")
	time.Sleep(3 * time.Second)
	// 生成唯一的 KAFKA_INSTANCE_ID
	instanceID := os.Getenv("KAFKA_INSTANCE_ID")
	if instanceID == "" {
		host, err := os.Hostname()
		if err != nil {
			host = "unknownhost"
		}
		timestamp := time.Now().UnixNano()
		instanceID = fmt.Sprintf("%s-%d", host, timestamp)
	}
	if opts.KafkaOptions != nil {
		opts.KafkaOptions.InstanceID = instanceID
		log.Infof("[Kafka] 自动生成唯一 InstanceID = %s", instanceID)
	}
	if err := g.initKafkaComponents(dbIns); err != nil {
		log.Error("kafka服务启动失败")
		return nil, err
	}
	log.Debug("kafka服务器启动成功")
	time.Sleep(3 * time.Second)

	// 启动消费者
	ctx, cancel := context.WithCancel(context.Background())
	g.consumerCtx = ctx
	g.consumerCancel = cancel

	// 获取所有消费者实例
	instances := g.getConsumerInstances()
	if instances != nil {
		// 启动所有消费者实例（每个实例1个worker）
		for i := 0; i < len(instances.createConsumers); i++ {
			if instances.createConsumers[i] != nil {
				go instances.createConsumers[i].StartConsuming(ctx, options.NewOptions().KafkaOptions.WorkerCount) // 每个实例1个worker
			}
			if instances.updateConsumers[i] != nil {
				go instances.updateConsumers[i].StartConsuming(ctx, 1)
			}
			if instances.deleteConsumers[i] != nil {
				go instances.deleteConsumers[i].StartConsuming(ctx, 1)
			}
		}

		// 单独启动重试消费者的所有实例，保证重试主题能在消费者组中均衡分配分区
		if len(instances.retryConsumers) > 0 {
			// 查询 topic 分区数用于指标和并发计算
			partitionCount := 0
			brokers := g.options.KafkaOptions.Brokers
			if len(brokers) > 0 {
				if p, err := getTopicPartitionCount(ctx, brokers, UserRetryTopic); err == nil {
					partitionCount = p
				} else {
					log.Warnf("无法获取 topic %s 的分区信息: %v", UserRetryTopic, err)
				}
			}

			// 更新 prometheus 指标
			retryGroupId := ConsumerGroupPrefix + "-retry"
			metrics.ConsumerTopicPartitions.WithLabelValues(UserRetryTopic).Set(float64(partitionCount))
			metrics.ConsumerGroupInstances.WithLabelValues(retryGroupId).Set(float64(len(instances.retryConsumers)))
			if len(instances.retryConsumers) == 0 {
				metrics.ConsumerPartitionsNoOwner.WithLabelValues(UserRetryTopic, retryGroupId).Set(float64(partitionCount))
			} else {
				// 简单启发式：当有实例存在时，认为无主分区为0（更精确的检测需要 Kafka admin/group 查询）
				metrics.ConsumerPartitionsNoOwner.WithLabelValues(UserRetryTopic, retryGroupId).Set(0)
			}

			// 根据分区数与实例数计算每个实例需要的 worker 数（上限为 RetryConsumerWorkers）
			workersPerInstance := 1
			if partitionCount > 0 && len(instances.retryConsumers) > 0 {
				workersPerInstance = (partitionCount + len(instances.retryConsumers) - 1) / len(instances.retryConsumers)
				if workersPerInstance > RetryConsumerWorkers {
					workersPerInstance = RetryConsumerWorkers
				}
				if workersPerInstance < 1 {
					workersPerInstance = 1
				}
			}

			for i := 0; i < len(instances.retryConsumers); i++ {
				if instances.retryConsumers[i] != nil {
					go instances.retryConsumers[i].StartConsuming(ctx, workersPerInstance)
				}
			}

			// 定期更新 topic/实例/无主分区指标（可配置）
			if g.options.KafkaOptions.EnableMetricsRefresh {
				go func() {
					ticker := time.NewTicker(g.options.KafkaOptions.MetricsRefreshInterval)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if len(brokers) == 0 {
								continue
							}

							// 更丰富的日志在 Debug 模式下打印
							isDebug := g.options.ServerRunOptions.Mode == "debug"

							if p, err := getTopicPartitionCount(ctx, brokers, UserRetryTopic); err == nil {
								metrics.ConsumerTopicPartitions.WithLabelValues(UserRetryTopic).Set(float64(p))
								metrics.ConsumerGroupInstances.WithLabelValues(retryGroupId).Set(float64(len(instances.retryConsumers)))
								if len(instances.retryConsumers) == 0 {
									metrics.ConsumerPartitionsNoOwner.WithLabelValues(UserRetryTopic, retryGroupId).Set(float64(p))
									if isDebug {
										log.Debugf("指标刷新: topic %s 分区=%d, instances=%d, noOwner=%d", UserRetryTopic, p, len(instances.retryConsumers), p)
									}
								} else {
									if noOwner, err := getPartitionsWithoutOwner(ctx, brokers, retryGroupId, UserRetryTopic); err == nil {
										metrics.ConsumerPartitionsNoOwner.WithLabelValues(UserRetryTopic, retryGroupId).Set(float64(noOwner))
										if isDebug {
											log.Debugf("指标刷新: topic %s 分区=%d, instances=%d, noOwner=%d", UserRetryTopic, p, len(instances.retryConsumers), noOwner)
										}
									} else {
										// 回退到启发式
										metrics.ConsumerPartitionsNoOwner.WithLabelValues(UserRetryTopic, retryGroupId).Set(0)
										log.Debugf("周期更新: 无法计算无主分区，使用回退值 0: %v", err)
									}
								}
							} else {
								if g.options.ServerRunOptions.Mode == "debug" {
									log.Debugf("周期更新: 无法读取 topic %s 分区信息: %v", UserRetryTopic, err)
								}
							}
						}
					}
				}()
			}
		}
		log.Debugf("已启动 %d 个消费者实例", len(instances.createConsumers))
	}

	time.Sleep(10 * time.Second) // 等待其他组件完全初始化
	// 如果我们未创建按实例存储（回退模式），启动单个全局重试消费者

	log.Debug("所有Kafka消费者已启动")
	g.printKafkaConfigInfo()

	//安装中间件
	if err := middleware.InstallMiddlewares(g.Engine, opts); err != nil {
		log.Error("中间件安装失败")
		return nil, err
	}
	log.Debug("中间件安装成功")

	//. 安装路由
	g.installRoutes()

	return g, nil
}

// ========== 新增：集群健康监控 ==========
func monitorClusterHealth(datastore *mysql.Datastore, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second // 默认30秒
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastStatus db.ClusterStatus
	unhealthyCount := 0

	for range ticker.C {
		currentStatus := datastore.ClusterStatus()

		// 只在状态变化时记录
		if currentStatus.PrimaryHealthy != lastStatus.PrimaryHealthy ||
			currentStatus.HealthyReplicas != lastStatus.HealthyReplicas {

			if currentStatus.PrimaryHealthy && currentStatus.HealthyReplicas > 0 {
				log.Debugf("📊 集群状态: 主节点健康，%d/%d 副本可用",
					currentStatus.HealthyReplicas, currentStatus.ReplicaCount)
				unhealthyCount = 0
			} else if !currentStatus.PrimaryHealthy {
				unhealthyCount++
				log.Errorf("🚨 集群告警: 主节点不可用 (连续%d次)", unhealthyCount)
			} else if currentStatus.HealthyReplicas == 0 {
				log.Warn("⚠️  集群警告: 无可用副本节点")
			}
		}

		lastStatus = currentStatus

		// 如果连续多次检测到主节点不可用，可能需要告警
		if unhealthyCount >= 3 {
			log.Error("🚨 严重: 集群主节点持续不可用，请立即检查!")
		}
	}
}

func (g *GenericAPIServer) configureGin() error {
	// 设置运行模式
	gin.SetMode(g.options.ServerRunOptions.Mode)

	// 开发环境配置
	if g.options.ServerRunOptions.Mode == gin.DebugMode {
		gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
			log.Debugf("📍 %-6s %-50s → %s (%d middleware)",
				httpMethod, absolutePath, filepath.Base(handlerName), nuHandlers)
		}
	} else {
		// 生产环境禁用调试输出
		gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {}
	}

	return nil
}

func (g *GenericAPIServer) Run() error {
	address := net.JoinHostPort(g.options.InsecureServingOptions.BindAddress, strconv.Itoa((g.options.InsecureServingOptions.BindPort)))

	g.insecureServer = &http.Server{
		Addr:    address,
		Handler: g,
		// 服务器性能优化
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,

		// 连接控制
		MaxHeaderBytes: 1 << 20, // 1MB
		// 新增：连接数限制
		ConnState: func(conn net.Conn, state http.ConnState) {
			// 监控连接状态，防止过多连接
		},
	}

	var eg errgroup.Group

	// 创建服务器启动信号通道
	serverStarted := make(chan struct{})

	eg.Go(func() error {
		log.Debugf("正在 %s 启动 GenericAPIServer 服务", address)

		// 创建监听器，确保端口可用
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("创建监听器失败: %w", err)
		}

		log.Debug("端口监听成功，开始接受连接")
		close(serverStarted)

		// 启动服务器
		err = g.insecureServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			log.Debugf("GenericAPIServer服务器已正常关闭")
			return nil
		}
		if err != nil {
			return fmt.Errorf("GenericAPIServer服务器启动失败: %w", err)
		}

		log.Debugf("停止 %s 运行的 GenericAPIServer 服务", address)
		return nil
	})

	// 等待服务器开始监听
	select {
	case <-serverStarted:
		log.Debug("GenericAPIServer服务器已开始监听，准备进行健康检查...")
	case <-time.After(10 * time.Second):
		return fmt.Errorf("GenericAPIServer服务器启动超时，无法在5秒内开始监听")
	}

	if g.options.ServerRunOptions.Healthz {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 先等待端口就绪
		if err := g.waitForPortReady(ctx, address, 10*time.Second); err != nil {
			return fmt.Errorf("端口就绪检测失败: %w", err)
		}

		// 执行健康检查
		if err := g.ping(ctx, address); err != nil {
			return fmt.Errorf("健康检查失败: %w", err)
		}
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("服务器运行错误: %w", err)
	}
	return nil
}

// waitForPortReady 等待端口就绪
func (g *GenericAPIServer) waitForPortReady(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	log.Debugf("等待端口 %s 就绪，超时时间: %v", address, timeout)

	for attempt := 1; ; attempt++ {
		// 检查是否超时
		if time.Now().After(deadline) {
			return fmt.Errorf("端口就绪检测超时")
		}

		// 尝试连接端口
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			log.Debugf("端口 %s 就绪检测成功，尝试次数: %d", address, attempt)
			return nil
		}

		// 记录重试信息（每5次尝试记录一次）
		if attempt%5 == 0 {
			log.Debugf("端口就绪检测尝试 %d: %v", attempt, err)
		}

		// 等待重试或上下文取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("端口就绪检测被取消: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
			// 继续重试
		}
	}
}

// 初始化Kafka组件
// internal/apiserver/server/server.go

// 初始化Kafka组件 - 使用options中的完整配置
func (g *GenericAPIServer) initKafkaComponents(db *gorm.DB) error {
	kafkaOpts := g.options.KafkaOptions

	// 1. 初始化生产者,消费者在处理消息时，可能需要将处理失败的消息发送到其他主题：
	log.Debug("初始化Kafka生产者...")
	userProducer := NewUserProducer(kafkaOpts)

	// 为每个主题创建多个消费者实例
	consumerCount := kafkaOpts.WorkerCount
	retryconsumerCount := kafkaOpts.RetryWorkerCount

	log.Debugf("为每个主题创建 %d 个消费者实例", consumerCount)

	// 创建消费者实例切片
	createConsumers := make([]*UserConsumer, consumerCount)
	updateConsumers := make([]*UserConsumer, consumerCount)
	deleteConsumers := make([]*UserConsumer, consumerCount)
	retryConsumers := make([]*RetryConsumer, retryconsumerCount)

	for i := 0; i < consumerCount; i++ {
		// 所有实例使用相同的消费组ID（不加后缀）
		createGroupID := ConsumerGroupPrefix + "-create" // 相同的组ID
		updateGroupID := ConsumerGroupPrefix + "-update" // 相同的组ID
		deleteGroupID := ConsumerGroupPrefix + "-delete" // 相同的组ID

		// 创建消费者实例 - 使用相同的消费组ID
		createConsumers[i] = NewUserConsumer(kafkaOpts, UserCreateTopic,
			createGroupID, db, g.redis) // ✅ 相同的组ID
		createConsumers[i].SetProducer(userProducer)
		createConsumers[i].SetInstanceID(i)

		updateConsumers[i] = NewUserConsumer(kafkaOpts, UserUpdateTopic,
			updateGroupID, db, g.redis) // ✅ 相同的组ID
		updateConsumers[i].SetProducer(userProducer)
		updateConsumers[i].SetInstanceID(i)

		deleteConsumers[i] = NewUserConsumer(kafkaOpts, UserDeleteTopic,
			deleteGroupID, db, g.redis) // ✅ 相同的组ID
		deleteConsumers[i].SetProducer(userProducer)
		deleteConsumers[i].SetInstanceID(i)
	}

	log.Debugf("初始化重试消费者...")
	retryGroupId := ConsumerGroupPrefix + "-retry"
	for i := 0; i < kafkaOpts.RetryWorkerCount; i++ {
		retryConsumers[i] = NewRetryConsumer(db, g.redis, userProducer, kafkaOpts, UserRetryTopic, retryGroupId)
	}
	// 3. 赋值到服务器实例
	g.producer = userProducer

	// 5. 存储所有消费者实例（新增字段）
	g.setConsumerInstances(createConsumers, updateConsumers, deleteConsumers, retryConsumers)

	return nil
}

func (g *GenericAPIServer) monitorRedisConnection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Debug("Redis集群监控退出")
			return
		case <-ticker.C:
			client := g.redis.GetClient()
			if client == nil {
				log.Error("Redis集群客户端丢失")
				continue
			}

			// 减少日志输出，只在出错时记录
			if err := g.pingRedis(ctx, client); err != nil {
				log.Errorf("Redis集群健康检查失败: %v", err)
			}
			// 成功时不输出日志，或者改为Debug级别
			// log.Debug("Redis集群健康检查通过")
		}
	}
}

func (g *GenericAPIServer) ping(ctx context.Context, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("无效的地址格式: %w", err)
	}

	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	url := fmt.Sprintf("http://%s/healthz", net.JoinHostPort(host, port))
	log.Debugf("开始健康检查，目标URL: %s", url)

	attempt := 0

	for {
		attempt++
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("健康检查超时: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("创建请求失败: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if attempt%3 == 0 { // 每3次失败记录一次日志，避免日志过多
				log.Debugf("健康检查尝试 %d 失败: %v", attempt, err)
			}
		} else {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Debug("健康检查成功")
				return nil
			}

			log.Debugf("健康检查尝试 %d: 状态码 %d", attempt, resp.StatusCode)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("健康检查超时: %w", ctx.Err())
		case <-time.After(1 * time.Second):
			// 继续重试
		}
	}
}

func (g *GenericAPIServer) initRedisStore() error {
	ctx, cancel := context.WithCancel(context.Background())
	g.redisCancel = cancel

	// 🔥 必须先初始化 g.redis！
	g.redis = &storage.RedisCluster{
		KeyPrefix: "genericapiserver:",
		HashKeys:  false,
		IsCache:   false,
	}

	// 启动异步连接任务
	go func() {
		log.Debugf("启动Redis集群异步连接任务")
		storage.ConnectToRedis(ctx, g.options.RedisOptions)
		log.Warn("Redis集群异步连接任务退出（可能上下文已取消）")
	}()

	// 同步等待Redis完全启动
	log.Debugf("等待Redis集群完全启动...")

	// 第一阶段：等待基础连接
	if err := g.waitForBasicConnection(10 * time.Second); err != nil {
		return err
	}

	// 第二阶段：等待健康检查通过
	if err := g.waitForHealthyCluster(ctx, 20*time.Second); err != nil {
		return err
	}

	log.Debug("✅ Redis集群完全启动并验证成功")

	// 启动监控
	go g.monitorRedisConnection(ctx)
	g.setupRedisClusterMonitoring()

	return nil
}

// 等待集群健康状态 - 添加 nil 检查
func (g *GenericAPIServer) waitForHealthyCluster(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for attempt := 1; time.Now().Before(deadline); attempt++ {
		// 🔥 添加 nil 检查
		if g.redis == nil {
			log.Warnf("RedisCluster实例为空（尝试 %d 次）", attempt)
			time.Sleep(2 * time.Second)
			continue
		}

		redisClient := g.redis.GetClient()
		if redisClient != nil {
			if err := g.pingRedis(ctx, redisClient); err == nil {
				log.Debugf("Redis集群健康检查通过（尝试 %d 次）", attempt)
				return nil
			}
		}

		if attempt%2 == 0 {
			log.Debugf("等待Redis集群健康检查...（尝试 %d 次）", attempt)
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("Redis集群健康检查超时（%v）", timeout)
}

// 等待基础连接建立 - 添加 nil 检查
func (g *GenericAPIServer) waitForBasicConnection(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for attempt := 1; time.Now().Before(deadline); attempt++ {
		// 🔥 添加 nil 检查
		if g.redis == nil {
			log.Warnf("RedisCluster实例为空（尝试 %d 次）", attempt)
			time.Sleep(1 * time.Second)
			continue
		}

		if storage.Connected() && g.redis.GetClient() != nil {
			log.Debugf("✅ Redis基础连接建立（尝试 %d 次）", attempt)
			return nil
		}

		if attempt%3 == 0 {
			log.Debugf("等待Redis基础连接...（尝试 %d 次）", attempt)
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("Redis基础连接建立超时（%v）", timeout)
}

// setupRedisClusterMonitoring 设置Redis集群监控
func (g *GenericAPIServer) setupRedisClusterMonitoring() {
	// 从Redis配置中获取集群节点地址
	nodes := g.options.RedisOptions.Addrs
	if len(nodes) == 0 {
		// 如果没有配置集群地址，使用默认的单节点地址
		nodes = []string{fmt.Sprintf("%s:%d", g.options.RedisOptions.Host, g.options.RedisOptions.Port)}
	}

	log.Debugf("启动Redis集群监控，节点: %v", nodes)

	// 创建集群监控器
	monitor := metrics.NewRedisClusterMonitor(
		"generic_api_server_cluster", // 集群名称
		nodes,                        // 集群节点地址
		30*time.Second,               // 每30秒采集一次
	)

	// 启动监控
	go monitor.Start(context.Background())

	log.Debug("✅ Redis集群监控已启动")
}

// pingRedis 支持redis.UniversalClient类型
func (g *GenericAPIServer) pingRedis(ctx context.Context, client redis.UniversalClient) error {
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 检查集群状态
	if clusterClient, ok := client.(*redis.ClusterClient); ok {
		// 集群模式：检查集群信息
		clusterInfo, err := clusterClient.ClusterInfo(pingCtx).Result()
		if err != nil {
			return fmt.Errorf("集群状态检查失败: %v", err)
		}

		// 执行 CLUSTER NODES 命令获取完整集群信息
		clusterNodes, err := clusterClient.ClusterNodes(pingCtx).Result()
		if err != nil {
			log.Warnf("执行CLUSTER NODES失败: %v", err)
		} else {
			//log.Infof("=== Redis集群节点详情 ===")
			//log.Infof("配置的节点数量: %d", len(g.options.RedisOptions.Addrs))
			//log.Infof("配置的节点列表: %v", g.options.RedisOptions.Addrs)

			lines := strings.Split(clusterNodes, "\n")
			actualNodeCount := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					actualNodeCount++
					//		log.Infof("节点 %d: %s", actualNodeCount, strings.TrimSpace(line))
				}
			}
			//		log.Infof("实际发现的节点数量: %d", actualNodeCount)
		}

		// 解析集群信息
		//log.Infof("=== Redis集群状态 ===")
		infoLines := strings.Split(clusterInfo, "\n")
		for _, line := range infoLines {
			if strings.Contains(line, ":") {
				//			log.Infof("  %s", strings.TrimSpace(line))
			}
		}

		// 🔥 修改：同时检查主节点和从节点
		var lastError error
		masterCount := 0
		slaveCount := 0
		successCount := 0

		// 检查所有主节点
		clusterClient.ForEachMaster(pingCtx, func(ctx context.Context, nodeClient *redis.Client) error {
			masterCount++
			if err := nodeClient.Ping(ctx).Err(); err != nil {
				log.Warnf("主节点 %d PING 失败: %v", masterCount, err)
				lastError = err
			} else {
				//			log.Infof("✅ 主节点 %d PING 成功", masterCount)
				successCount++
			}
			return nil
		})

		// 检查所有从节点
		err = clusterClient.ForEachSlave(pingCtx, func(ctx context.Context, nodeClient *redis.Client) error {
			slaveCount++
			if err := nodeClient.Ping(ctx).Err(); err != nil {
				log.Warnf("从节点 %d PING 失败: %v", slaveCount, err)
				lastError = err
			} else {
				//		log.Infof("✅ 从节点 %d PING 成功", slaveCount)
				successCount++
			}
			return nil
		})

		totalNodes := masterCount + slaveCount

		// log.Infof("=== Redis集群健康检查总结 ===")
		// log.Infof("主节点数: %d, 从节点数: %d", masterCount, slaveCount)
		// log.Infof("总节点数: %d, 成功节点: %d, 失败节点: %d", totalNodes, successCount, totalNodes-successCount)

		if successCount == 0 {
			return fmt.Errorf("所有集群节点PING检查失败")
		}

		// 🔥 修改：检查是否所有配置的节点都被发现
		expectedNodes := len(g.options.RedisOptions.Addrs)
		if totalNodes != expectedNodes {
			log.Warnf("⚠️  节点数量不匹配: 配置%d个, 集群中发现%d个", expectedNodes, totalNodes)
		} else {
			//		log.Infof("✅ 节点数量匹配: %d个", totalNodes)
		}

		if successCount < totalNodes {
			log.Warnf("部分节点连接异常 (%d/%d 成功)", successCount, totalNodes)
		} else {
			//		log.Infof("✅ 所有节点连接正常")
		}

		// 如果至少有一个节点正常，认为集群可用
		if err != nil && lastError != nil {
			return fmt.Errorf("集群节点检查异常: %v", lastError)
		}

		return nil
	}

	// 单机模式或普通客户端
	return client.Ping(pingCtx).Err()
}

// 打印Kafka配置信息
func (g *GenericAPIServer) printKafkaConfigInfo() {
	kafkaOpts := g.options.KafkaOptions
	instances := g.getConsumerInstances()
	instanceCount := 1
	if instances != nil {
		instanceCount = len(instances.createConsumers)
	}

	log.Debugf("📊 Kafka配置信息:")
	log.Debugf("  运行模式: %s", g.options.ServerRunOptions.Mode)
	log.Debugf("  Brokers: %v", kafkaOpts.Brokers)
	log.Debugf("  主题配置:")
	log.Debugf("    - 创建: %s (%d个消费者实例)", UserCreateTopic, instanceCount)
	log.Debugf("    - 更新: %s (%d个消费者实例)", UserUpdateTopic, instanceCount)
	log.Debugf("    - 删除: %s (%d个消费者实例)", UserDeleteTopic, instanceCount)
	log.Debugf("    - 重试: %s", UserRetryTopic)
	log.Debugf("  配置参数:")
	log.Debugf("    - 最大重试: %d", kafkaOpts.MaxRetries)
	log.Debugf("    - 消费者实例数量: %d", instanceCount)
	log.Debugf("    - 批量大小: %d", kafkaOpts.BatchSize)
	log.Debugf("    - 批量超时: %v", kafkaOpts.BatchTimeout)
}

// 新增：存储所有消费者实例
type consumerInstances struct {
	createConsumers []*UserConsumer
	updateConsumers []*UserConsumer
	deleteConsumers []*UserConsumer
	retryConsumers  []*RetryConsumer
}

var consumerInstancesStore = &consumerInstances{}

func (g *GenericAPIServer) setConsumerInstances(create, update, delete []*UserConsumer, retry []*RetryConsumer) {
	consumerInstancesStore.createConsumers = create
	consumerInstancesStore.updateConsumers = update
	consumerInstancesStore.deleteConsumers = delete
	consumerInstancesStore.retryConsumers = retry

}

func (g *GenericAPIServer) getConsumerInstances() *consumerInstances {
	return consumerInstancesStore
}

// ========== 新增：集群初始化函数 ==========
func initializeGaleraCluster(datastore *mysql.Datastore) error {
	maxRetries := 20                 // 最大重试次数
	retryInterval := 2 * time.Second // 重试间隔

	for attempt := 1; attempt <= maxRetries; attempt++ {
		status := datastore.ClusterStatus()

		log.Debugf("🔍 集群健康检查 [%d/%d]: 主节点=%v, 副本=%d/%d 健康",
			attempt, maxRetries, status.PrimaryHealthy, status.HealthyReplicas, status.ReplicaCount)

		// 检查集群健康条件
		if status.PrimaryHealthy {
			if status.HealthyReplicas >= 1 {
				// 理想状态：主节点健康且至少1个副本健康
				log.Debugf("✅ Galera集群状态良好: 主节点健康，%d个副本节点可用", status.HealthyReplicas)
				return nil
			} else if status.HealthyReplicas == 0 {
				// 只有主节点健康（可能是单节点集群或副本节点故障）
				log.Warn("⚠️  Galera集群主节点健康，但副本节点不可用")
				return nil // 仍然继续启动
			}
		}

		if attempt < maxRetries {
			log.Debugf("⏳ 集群未就绪，%v后重试...", retryInterval)
			time.Sleep(retryInterval)
		}
	}

	// 最终检查
	finalStatus := datastore.ClusterStatus()
	if !finalStatus.PrimaryHealthy {
		return fmt.Errorf("Galera集群主节点不可用，请检查集群状态")
	}

	log.Warn("⚠️  Galera集群部分节点不可用，但服务将继续启动")
	return nil
}

// getTopicPartitionCount returns the number of partitions for the given topic using kafka.Client.Metadata
func getTopicPartitionCount(ctx context.Context, brokers []string, topic string) (int, error) {
	if len(brokers) == 0 {
		return 0, fmt.Errorf("no brokers provided")
	}

	admin := &kafka.Client{Addr: kafka.TCP(brokers...)}
	metadata, err := admin.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return 0, err
	}

	for _, t := range metadata.Topics {
		if t.Name == topic {
			return len(t.Partitions), nil
		}
	}
	return 0, fmt.Errorf("topic %s not found in metadata", topic)
}

// getPartitionsWithoutOwner queries the consumer group and topic metadata to compute the number
// of partitions of 'topic' that are not currently assigned to any member of the consumer group.
// It uses kafka.Client to fetch Metadata and DescribeGroups.
func getPartitionsWithoutOwner(ctx context.Context, brokers []string, groupID, topic string) (int, error) {
	if len(brokers) == 0 {
		return 0, fmt.Errorf("no brokers provided")
	}

	admin := &kafka.Client{Addr: kafka.TCP(brokers...)}

	// 1) 获取 topic partitions
	metadata, err := admin.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return 0, fmt.Errorf("metadata error: %w", err)
	}
	var topicMeta *kafka.Topic
	for _, t := range metadata.Topics {
		if t.Name == topic {
			topicMeta = &t
			break
		}
	}
	if topicMeta == nil {
		return 0, fmt.Errorf("topic %s not found", topic)
	}
	totalPartitions := len(topicMeta.Partitions)

	// 2) Describe group to get member assignments
	describeResp, err := admin.DescribeGroups(ctx, &kafka.DescribeGroupsRequest{GroupIDs: []string{groupID}})
	if err != nil {
		return 0, fmt.Errorf("describe groups error: %w", err)
	}
	if len(describeResp.Groups) == 0 {
		// 没有成员，所有分区都没有 owner
		return totalPartitions, nil
	}

	// Collect partitions that are owned by members (for the topic)
	owned := make(map[int]struct{})
	for _, g := range describeResp.Groups {
		for _, member := range g.Members {
			// Use MemberAssignments (Topics/Partitions)
			for _, t := range member.MemberAssignments.Topics {
				if t.Topic != topic {
					continue
				}
				for _, p := range t.Partitions {
					owned[p] = struct{}{}
				}
			}
			// Also include OwnedPartitions from MemberMetadata for cooperative assignor
			for _, op := range member.MemberMetadata.OwnedPartitions {
				if op.Topic != topic {
					continue
				}
				for _, p := range op.Partitions {
					owned[p] = struct{}{}
				}
			}
		}
	}

	// If we couldn't find owned partitions via DescribeGroups parsing, fallback to 0 ownership (conservative)
	if len(owned) == 0 {
		// Fallback: use ConsumerOffsets (deprecated helper) to see committed offsets for group/topic
		if offs, err := admin.ConsumerOffsets(ctx, kafka.TopicAndGroup{Topic: topic, GroupId: groupID}); err == nil {
			for pid := range offs {
				owned[pid] = struct{}{}
			}
		}
	}

	// Count partitions without owner
	noOwner := 0
	for _, p := range topicMeta.Partitions {
		if _, ok := owned[p.ID]; !ok {
			noOwner++
		}
	}

	return noOwner, nil
}
