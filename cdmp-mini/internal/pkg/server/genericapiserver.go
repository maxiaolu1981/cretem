package server

import (
	"context"
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

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	mysql "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"

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
	createConsumer *UserConsumer
	updateConsumer *UserConsumer
	deleteConsumer *UserConsumer
	retryConsumer  *RetryConsumer
}

func NewGenericAPIServer(opts *options.Options) (*GenericAPIServer, error) {
	// 初始化日志
	log.Infof("正在初始化GenericAPIServer服务器，环境: %s", opts.ServerRunOptions.Mode)

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

	//初始化mysql
	storeIns, dbIns, err := mysql.GetMySQLFactoryOr(opts.MysqlOptions)
	if err != nil {
		log.Error("mysql服务器启动失败")
		return nil, err
	}
	interfaces.SetClient(storeIns)
	log.Info("mysql服务器初始化成功")
	time.Sleep(3 * time.Second)

	//初始化redis
	if err := g.initRedisStore(); err != nil {
		log.Error("redis服务器启动失败")
		return nil, err
	}
	log.Info("redis服务器启动成功")
	time.Sleep(3 * time.Second)

	// 初始化Kafka生产者和消费者
	if err := InitKafkaWithRetry(opts); err != nil {
		log.Error("kafka测试连通失败")
		return nil, errors.WithCode(code.ErrKafkaFailed, "kafka服务未启动")
	}

	if err := g.initKafkaComponents(dbIns); err != nil {
		log.Error("kafka服务启动失败")
		return nil, err
	}
	log.Info("kafka服务器启动成功")
	time.Sleep(3 * time.Second)

	// 启动消费者
	ctx, cancel := context.WithCancel(context.Background())
	g.consumerCtx = ctx
	g.consumerCancel = cancel
	// 启动消费者，使用配置的WorkerCount
	mainWorkers := opts.KafkaOptions.WorkerCount
	if mainWorkers <= 0 {
		mainWorkers = MainConsumerWorkers // 使用默认值
	}

	go g.createConsumer.StartConsuming(ctx, mainWorkers)
	go g.updateConsumer.StartConsuming(ctx, mainWorkers)
	go g.deleteConsumer.StartConsuming(ctx, mainWorkers)
	go g.retryConsumer.StartConsuming(ctx, RetryConsumerWorkers)

	log.Info("所有Kafka消费者已启动")
	g.printKafkaConfigInfo()
	


	//安装中间件
	if err := middleware.InstallMiddlewares(g.Engine, opts); err != nil {
		log.Error("中间件安装失败")
		return nil, err
	}
	log.Info("中间件安装成功")

	//. 安装路由
	g.installRoutes()

	return g, nil
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
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
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
		log.Infof("正在 %s 启动 GenericAPIServer 服务", address)

		// 创建监听器，确保端口可用
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("创建监听器失败: %w", err)
		}

		log.Info("端口监听成功，开始接受连接")
		close(serverStarted)

		// 启动服务器
		err = g.insecureServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			log.Infof("GenericAPIServer服务器已正常关闭")
			return nil
		}
		if err != nil {
			return fmt.Errorf("GenericAPIServer服务器启动失败: %w", err)
		}

		log.Infof("停止 %s 运行的 GenericAPIServer 服务", address)
		return nil
	})

	// 等待服务器开始监听
	select {
	case <-serverStarted:
		log.Info("GenericAPIServer服务器已开始监听，准备进行健康检查...")
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
	log.Infof("等待端口 %s 就绪，超时时间: %v", address, timeout)

	for attempt := 1; ; attempt++ {
		// 检查是否超时
		if time.Now().After(deadline) {
			return fmt.Errorf("端口就绪检测超时")
		}

		// 尝试连接端口
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			log.Infof("端口 %s 就绪检测成功，尝试次数: %d", address, attempt)
			return nil
		}

		// 记录重试信息（每5次尝试记录一次）
		if attempt%5 == 0 {
			log.Infof("端口就绪检测尝试 %d: %v", attempt, err)
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

	log.Infof("初始化Kafka组件，最大重试: %d, Worker数量: %d",
		kafkaOpts.MaxRetries, kafkaOpts.WorkerCount)

	// 1. 初始化生产者
	log.Info("初始化Kafka生产者...")
	userProducer := NewUserProducer(kafkaOpts)

	// 2. 初始化各个主题的消费者
	consumerGroupPrefix := "user-service"
	if g.options.ServerRunOptions.Mode == gin.ReleaseMode {
		consumerGroupPrefix = "user-service-prod"
	}

	log.Info("初始化创建消费者...")
	createConsumer := NewUserConsumer(kafkaOpts.Brokers, UserCreateTopic,
		consumerGroupPrefix+"-create", db, g.redis)
	//createConsumer.SetKafkaOptions(kafkaOpts) // 传递完整配置
	createConsumer.SetProducer(userProducer)

	log.Info("初始化更新消费者...")
	updateConsumer := NewUserConsumer(kafkaOpts.Brokers, UserUpdateTopic,
		consumerGroupPrefix+"-update", db, g.redis)
	//updateConsumer.SetKafkaOptions(kafkaOpts)
	updateConsumer.SetProducer(userProducer)

	log.Info("初始化删除消费者...")
	deleteConsumer := NewUserConsumer(kafkaOpts.Brokers, UserDeleteTopic,
		consumerGroupPrefix+"-delete", db, g.redis)
	//deleteConsumer.SetKafkaOptions(kafkaOpts)
	deleteConsumer.SetProducer(userProducer)

	log.Info("初始化重试消费者...")
	retryConsumer := NewRetryConsumer(db, g.redis, userProducer, kafkaOpts)
	//retryConsumer.SetKafkaOptions(kafkaOpts) // 传递配置给重试消费者

	// 3. 赋值到服务器实例
	g.producer = userProducer
	g.createConsumer = createConsumer
	g.updateConsumer = updateConsumer
	g.deleteConsumer = deleteConsumer
	g.retryConsumer = retryConsumer

	log.Infof("✅ Kafka组件初始化完成，配置: 重试%d次, Worker%d个, 批量%d, 超时%v",
		kafkaOpts.MaxRetries, kafkaOpts.WorkerCount, kafkaOpts.BatchSize, kafkaOpts.BatchTimeout)
	return nil
}

func (g *GenericAPIServer) monitorRedisConnection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Redis集群监控退出")
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
	log.Infof("开始健康检查，目标URL: %s", url)

	startTime := time.Now()
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
				log.Infof("健康检查尝试 %d 失败: %v", attempt, err)
			}
		} else {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Infof("健康检查成功! 总共尝试 %d 次, 耗时 %v",
					attempt, time.Since(startTime))
				return nil
			}

			log.Infof("健康检查尝试 %d: 状态码 %d", attempt, resp.StatusCode)
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
		log.Info("启动Redis集群异步连接任务")
		storage.ConnectToRedis(ctx, g.options.RedisOptions)
		log.Warn("Redis集群异步连接任务退出（可能上下文已取消）")
	}()

	// 同步等待Redis完全启动
	log.Info("等待Redis集群完全启动...")

	// 第一阶段：等待基础连接
	if err := g.waitForBasicConnection(10 * time.Second); err != nil {
		return err
	}

	// 第二阶段：等待健康检查通过
	if err := g.waitForHealthyCluster(ctx, 20*time.Second); err != nil {
		return err
	}

	log.Info("✅ Redis集群完全启动并验证成功")

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
				log.Infof("✅ Redis集群健康检查通过（尝试 %d 次）", attempt)
				return nil
			}
		}

		if attempt%2 == 0 {
			log.Infof("等待Redis集群健康检查...（尝试 %d 次）", attempt)
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
			log.Infof("✅ Redis基础连接建立（尝试 %d 次）", attempt)
			return nil
		}

		if attempt%3 == 0 {
			log.Infof("等待Redis基础连接...（尝试 %d 次）", attempt)
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

	log.Infof("启动Redis集群监控，节点: %v", nodes)

	// 创建集群监控器
	monitor := metrics.NewRedisClusterMonitor(
		"generic_api_server_cluster", // 集群名称
		nodes,                        // 集群节点地址
		30*time.Second,               // 每30秒采集一次
	)

	// 启动监控
	go monitor.Start(context.Background())

	log.Info("✅ Redis集群监控已启动")
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
		err = clusterClient.ForEachMaster(pingCtx, func(ctx context.Context, nodeClient *redis.Client) error {
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
	log.Infof("📊 Kafka配置信息:")
	log.Infof("  运行模式: %s", g.options.ServerRunOptions.Mode)
	log.Infof("  Brokers: %v", kafkaOpts.Brokers)
	log.Infof("  主题配置:")
	log.Infof("    - 创建: %s", UserCreateTopic)
	log.Infof("    - 更新: %s", UserUpdateTopic)
	log.Infof("    - 删除: %s", UserDeleteTopic)
	log.Infof("    - 重试: %s", UserRetryTopic)
	log.Infof("  配置参数:")
	log.Infof("    - 最大重试: %d", kafkaOpts.MaxRetries)
	log.Infof("    - Worker数量: %d", kafkaOpts.WorkerCount)
	log.Infof("    - 批量大小: %d", kafkaOpts.BatchSize)
	log.Infof("    - 批量超时: %v", kafkaOpts.BatchTimeout)
	log.Infof("    - 确认机制: %d", kafkaOpts.RequiredAcks)
}
