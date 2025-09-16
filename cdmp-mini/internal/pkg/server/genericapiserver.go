package server

import (
	"context"
	"sync"

	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	mysql "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// 全局变量存储Redis客户端（用于监控）
var redisClient redis.UniversalClient

// 新增：存储分布式锁开关状态
var distributedLockEnabled bool

type GenericAPIServer struct {
	insecureServer *http.Server
	*gin.Engine
	options        *options.Options
	redis          *storage.RedisCluster
	redisCancel    context.CancelFunc
	BloomFilter    *bloom.BloomFilter
	BloomMutex     sync.RWMutex
	initOnce       sync.Once
	initErr        error
	producer       *Producer
	consumerCtx    context.Context
	consumerCancel context.CancelFunc
}

func NewGenericAPIServer(opts *options.Options) (*GenericAPIServer, error) {
	// 初始化日志
	log.Infof("正在初始化GenericAPIServer服务器，环境: %s", opts.ServerRunOptions.Mode)

	//创建服务器实例
	g := &GenericAPIServer{
		Engine:      gin.New(),
		options:     opts,
		BloomFilter: bloom.NewWithEstimates(1000000, 0.001),
		BloomMutex:  sync.RWMutex{},
		initOnce:    sync.Once{},
	}

	//设置gin运行模式
	if err := g.configureGin(); err != nil {
		return nil, err
	}

	//初始化mysql
	log.Info("正在初始化mysql服务器")
	storeIns, dbIns, err := mysql.GetMySQLFactoryOr(opts.MysqlOptions)
	if err != nil {
		return nil, err
	}
	interfaces.SetClient(storeIns)
	log.Info("mysql服务器初始化成功")

	//初始化redis
	log.Info("正在初始化redis服务器")
	if err := g.initRedisStore(); err != nil {
		log.Error("初始化redis服务器失败")
		return nil, err
	}
	log.Info("redis服务器初始化成功")

	//初始化boolm
	log.Info("正在初始化boolm服务")
	g.initBloomFiliter()
	if g.initErr != nil {
		log.Warnf("初始化boolm失败%v", g.initErr)
	}
	log.Info("初始化boolm服务成功")

	// 初始化Kafka生产者和消费者（同时启动！）
	producer, consumer := g.initKafkaComponents(g.options.KafkaOptions.Brokers,
		"user-create-topic", "user-group", dbIns)
	g.producer = producer

	// 4. 启动Kafka消费者（后台运行）
	ctx, cancel := context.WithCancel(context.Background())
	g.consumerCtx = ctx
	g.consumerCancel = cancel
	go startKafkaConsumer(ctx, consumer, 5) // 启动5个消费者worker

	//安装中间件
	if err := middleware.InstallMiddlewares(g.Engine, opts); err != nil {
		return nil, err
	}
	//. 安装路由
	g.installRoutes()

	return g, nil
}

// 启动Kafka消费者
func startKafkaConsumer(ctx context.Context, consumer *Consumer, workerCount int) {
	log.Infof("开始运行kafka消费者%d workers...", workerCount)
	// 这里会阻塞运行，直到context被取消
	consumer.StartConsuming(ctx, workerCount)

}

func (g *GenericAPIServer) initBloomFiliter() error {
	// 使用sync.Once确保只执行一次初始化

	g.initOnce.Do(func() {
		g.BloomMutex.Lock()
		defer g.BloomMutex.Unlock()
		// 从数据库加载所有用户名
		users, err := interfaces.Client().Users().ListAllUsernames(context.TODO())
		if err != nil {
			g.initErr = errors.WithCode(code.ErrUnknown, "创建Bloom错误%v", err)
			return
		}
		if len(users) == 0 {
			log.Debug("目前没有任何用户记录,未初始化布隆过滤器")
			return
		}

		for _, name := range users {
			g.BloomFilter.AddString(name)
		}

		log.Debugf("Bloom filter initialized with %d usernames", len(users))
	})
	return nil
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
func (g *GenericAPIServer) initKafkaComponents(brokers []string, topic, groupID string, db *gorm.DB) (*Producer, *Consumer) {
	// 初始化生产者
	producer := g.NewKafkaProducer(brokers, topic)
	log.Infof("初始化kafka生产者: %s", topic)

	// 初始化消费者（注入数据库连接）
	consumer := NewKafkaConsumer(brokers, topic, groupID, db)
	log.Infof("初始化kafka消费者: %s, group: %s", topic, groupID)

	return producer, consumer
}

// initRedisStore 初始化Redis存储，根据分布式锁开关状态调整初始化策略
// 在initRedisStore函数中正确初始化RedisCluster
func (g *GenericAPIServer) initRedisStore() error {
	// 获取分布式锁开关状态
	distributedLockEnabled = g.options.DistributedLock.Enabled
	log.Infof("分布式锁开关状态: %v", distributedLockEnabled)

	ctx, cancel := context.WithCancel(context.Background())
	g.redisCancel = cancel
	defer func() {
		if r := recover(); r != nil || redisClient == nil {
			cancel()
			log.Errorf("Redis初始化异常，触发上下文取消: recover=%v, 客户端是否为空=%t", r, redisClient == nil)
		}
	}()

	// 关键修复：正确初始化RedisCluster，明确设置IsCache=false
	g.redis = &storage.RedisCluster{
		KeyPrefix: "genericapiserver:",
		HashKeys:  false,
		IsCache:   false, // 匹配singlePool（非缓存客户端）
	}
	log.Debugf("RedisCluster实例初始化完成，KeyPrefix=%s，IsCache=%v", g.redis.KeyPrefix, g.redis.IsCache)

	// 启动Redis异步连接任务
	go func() {
		log.Info("启动Redis异步连接任务")
		storage.ConnectToRedis(ctx, g.options.RedisOptions)
		log.Warn("Redis异步连接任务退出（可能上下文已取消）")
	}()

	// 等待Redis客户端就绪（带重试机制）
	const (
		maxRetries    = 30
		retryInterval = 1 * time.Second
	)
	var retryCount int
	var lastErr error

	for {
		// 检查存储是否标记为已连接
		if !storage.Connected() {
			lastErr = fmt.Errorf("storage未标记为已连接")
		} else {
			// 尝试获取客户端并验证可用性
			redisClient = g.redis.GetClient()
			if redisClient != nil {
				if err := pingRedis(ctx, redisClient); err == nil {
					log.Info("✅ Redis客户端获取成功且验证可用")
					go g.monitorRedisConnection(ctx)
					return nil
				} else {
					lastErr = fmt.Errorf("客户端非空但验证失败: %v", err)
					redisClient = nil // 重置客户端，重新尝试
				}
			} else {
				lastErr = fmt.Errorf("GetClient()返回空客户端")
			}
		}

		// 检查是否达到最大重试次数
		retryCount++
		if retryCount >= maxRetries {
			if distributedLockEnabled {
				finalErr := fmt.Errorf("Redis初始化失败（核心依赖），已达最大重试次数(%d)，最后错误: %v", maxRetries, lastErr)
				log.Fatal(finalErr.Error())
				return finalErr
			} else {
				log.Warnf("Redis初始化重试达最大次数(%d)（非核心依赖），最后错误: %v，继续启动并监控", maxRetries, lastErr)
				go g.monitorRedisConnection(ctx)
				return nil
			}
		}

		// 输出重试日志
		if distributedLockEnabled {
			log.Warnf("Redis初始化重试中（核心依赖），第%d/%d次，最后错误: %v", retryCount, maxRetries, lastErr)
		} else {
			log.Debugf("Redis初始化重试中（非核心依赖），第%d/%d次，最后错误: %v", retryCount, maxRetries, lastErr)
		}
		time.Sleep(retryInterval)
	}
}

// monitorRedisConnection 监控Redis连接状态，根据分布式锁开关决定是否影响主进程
func (g *GenericAPIServer) monitorRedisConnection(ctx context.Context) {
	lastConnected := true
	monitorTicker := time.NewTicker(3 * time.Second)
	defer monitorTicker.Stop()

	// 根据锁开关状态输出监控启动日志
	if distributedLockEnabled {
		log.Info("🔍 Redis运行期监控协程已启动（分布式锁启用，连接中断将导致进程退出）")
	} else {
		log.Info("🔍 Redis运行期监控协程已启动（分布式锁禁用，连接中断不影响主进程）")
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("📌 Redis监控协程因上下文取消而退出")
			return
		case <-monitorTicker.C:
			err := pingRedis(ctx, redisClient)
			current := err == nil

			// 处理连接状态变化
			if current && !lastConnected {
				log.Info("📈 Redis连接已恢复")
				lastConnected = current
			} else if !current && lastConnected {
				log.Error("📉 Redis连接已断开，尝试重连...")

				// 尝试重连3次
				reconnectSuccess := false
				for i := 0; i < 3; i++ {
					time.Sleep(1 * time.Second)
					if err := pingRedis(ctx, redisClient); err == nil {
						reconnectSuccess = true
						break
					}
				}

				if !reconnectSuccess {
					if distributedLockEnabled {
						log.Fatal("❌ Redis多次重连失败（分布式锁启用，核心依赖不可用），主进程将退出")
						os.Exit(137)
					} else {
						log.Warn("⚠️ Redis多次重连失败（分布式锁禁用，非核心依赖），继续监控")
						lastConnected = false
					}
				} else {
					log.Info("📈 Redis重连成功")
					lastConnected = true
				}
			}
		}
	}
}

// 修正：添加 ctx context.Context 参数，适配 v8 的 Ping 方法签名
func pingRedis(ctx context.Context, client redis.UniversalClient) error {
	if client == nil {
		return fmt.Errorf("Redis客户端未初始化")
	}

	resultChan := make(chan error, 1)

	// 异步执行PING命令，避免阻塞
	go func() {
		var pingCmd *redis.StatusCmd

		// 类型断言，适配不同客户端类型（v8 需传 ctx）
		switch c := client.(type) {
		case *redis.Client:
			pingCmd = c.Ping(ctx) // 修正：传入 ctx
		case *redis.ClusterClient:
			pingCmd = c.Ping(ctx) // 修正：传入 ctx
		default:
			resultChan <- fmt.Errorf("不支持的Redis客户端类型: %T", client)
			return
		}

		// 检查命令执行结果
		if pingCmd.Err() != nil {
			resultChan <- handlePingError(pingCmd.Err())
			return
		}

		// 验证响应内容
		if pingCmd.Val() != "PONG" {
			resultChan <- fmt.Errorf("redis ping响应异常: %s", pingCmd.Val())
			return
		}

		resultChan <- nil
	}()

	// 超时控制（2秒）：结合外部ctx和超时
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case err := <-resultChan:
		return err
	case <-timeoutCtx.Done():
		return fmt.Errorf("PING命令超时（超过2秒）: %w", timeoutCtx.Err())
	}
}

// handlePingError 分类处理PING命令的错误
func handlePingError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "i/o timeout") ||
		strings.Contains(err.Error(), "closed") {
		return fmt.Errorf("连接失败: %v", err)
	}
	return fmt.Errorf("PING失败: %v", err)
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
