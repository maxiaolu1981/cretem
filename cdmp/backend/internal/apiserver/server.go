// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
/*
包摘要
当前包为 apiserver，是一个 API 服务器的核心实现，主要功能包括：
初始化并启动通用 API 服务器（genericAPIServer）和 gRPC 服务器（gRPCAPIServer）
管理 MySQL、Redis 等存储的连接与关闭
实现服务的优雅启动与关闭（通过优雅关闭管理器）
注册 gRPC 服务（如缓存相关服务）并配置 TLS 安全认证
核心流程分析
整个 API 服务器的生命周期可分为 初始化→准备→运行→关闭 四个阶段，具体流程如下：
初始化阶段（createAPIServer）
创建优雅关闭管理器（shutdown.GracefulShutdown），并注册 POSIX 信号管理器（用于处理系统退出信号，如SIGINT）。
从配置（config.Config）中构建通用 API 服务器配置（genericConfig）和 gRPC 服务器额外配置（extraConfig）。
基于配置初始化通用 API 服务器（genericAPIServer）和 gRPC 服务器（gRPCAPIServer），并封装为apiServer实例。
准备阶段（PrepareRun）
初始化通用 API 服务器的路由（initRouter，需外部实现）。
初始化 Redis 连接（initRedisStore），并设置 Redis 连接的关闭回调（通过优雅关闭管理器）。
注册服务关闭回调：在服务退出时，关闭 MySQL 连接、gRPC 服务器和通用 API 服务器，确保资源释放。
运行阶段（Run）
启动 gRPC 服务器（通过 goroutine 异步运行，避免阻塞）。
启动优雅关闭管理器，开始监听系统退出信号。
启动通用 API 服务器（阻塞当前进程，确保服务持续运行）。
关闭阶段（优雅关闭）
当接收到系统退出信号（如Ctrl+C）时，优雅关闭管理器触发注册的回调函数：
关闭 Redis 连接（通过cancel终止 Redis 连接上下文）
关闭 MySQL 连接
停止 gRPC 服务器和通用 API 服务器
*/
package apiserver

import (
	"context"
	"fmt" // 标准库：Go 内置基础包

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials" // 第三方库：gRPC 官方提供的认证包
	"google.golang.org/grpc/reflection"  // 第三方库：gRPC 反射服务（用于调试）

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/config"                      // 自定义库：API 服务器配置
	cachev1 "github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/controller/v1/cache" // 自定义库：缓存控制器实现
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"                       // 自定义库：存储客户端管理
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store/mysql"                 // 自定义库：MySQL 存储实现
	genericoptions "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options"            // 自定义库：通用配置选项
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"           // 自定义库：通用 API 服务器框架
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"                                        // 自定义库：日志工具
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/shutdown"                                   // 自定义库：优雅关闭管理器
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/shutdown/shutdownmanagers/posixsignal"      // 自定义库：POSIX 信号处理
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/storage"                                    // 自定义库：存储连接工具（如 Redis）
	pb "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"                               // 自定义库：gRPC 生成的接口定义
)

// apiServer 是API服务器的核心结构体，整合所有组件
type apiServer struct {
	gs               *shutdown.GracefulShutdown         // 优雅关闭管理器（处理服务启动/关闭）
	redisOptions     *genericoptions.RedisOptions       // Redis配置选项
	gRPCAPIServer    *grpcAPIServer                     // gRPC服务器实例
	genericAPIServer *genericapiserver.GenericAPIServer // 通用API服务器实例（如HTTP服务器）
}

// preparedAPIServer 是准备就绪的API服务器，封装apiServer并提供Run方法
type preparedAPIServer struct {
	*apiServer
}

// ExtraConfig 定义gRPC服务器的额外配置（通用API服务器之外的配置）
type ExtraConfig struct {
	Addr         string                            // gRPC服务器监听地址（如"127.0.0.1:8081"）
	MaxMsgSize   int                               // gRPC最大消息大小
	ServerCert   genericoptions.GeneratableKeyCert // TLS证书配置（用于gRPC安全认证）
	mysqlOptions *genericoptions.MySQLOptions      // MySQL配置选项（用于初始化MySQL连接）
	// etcdOptions      *genericoptions.EtcdOptions        // 注释：Etcd配置选项（预留）
}

// createAPIServer 基于配置创建apiServer实例
func createAPIServer(cfg *config.Config) (*apiServer, error) {
	// 初始化优雅关闭管理器，并注册POSIX信号管理器（监听系统退出信号）
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	// 构建通用API服务器配置
	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 构建gRPC服务器的额外配置
	extraConfig, err := buildExtraConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 基于配置创建通用API服务器实例
	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}
	// 基于额外配置创建gRPC服务器实例
	extraServer, err := extraConfig.complete().New()
	if err != nil {
		return nil, err
	}

	// 封装所有组件为apiServer实例
	server := &apiServer{
		gs:               gs,
		redisOptions:     cfg.RedisOptions,
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
	}

	return server, nil
}

// PrepareRun 准备服务运行环境（初始化路由、存储连接等），返回准备就绪的服务器
func (s *apiServer) PrepareRun() preparedAPIServer {
	// 初始化通用API服务器的路由（需外部实现具体路由逻辑）
	initRouter(s.genericAPIServer.Engine)

	// 初始化Redis存储连接
	s.initRedisStore()

	// 注册服务关闭回调：在服务退出时释放资源
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		// 关闭MySQL连接
		mysqlStore, _ := mysql.GetMySQLFactoryOr(nil)
		if mysqlStore != nil {
			_ = mysqlStore.Close()
		}

		// 关闭gRPC服务器和通用API服务器
		s.gRPCAPIServer.Close()
		s.genericAPIServer.Close()

		return nil
	}))

	return preparedAPIServer{s}
}

// Run 启动API服务器（阻塞方法，确保服务持续运行）
func (s preparedAPIServer) Run() error {
	// 异步启动gRPC服务器（避免阻塞通用API服务器启动）
	go s.gRPCAPIServer.Run()

	// 启动优雅关闭管理器，开始监听退出信号
	if err := s.gs.Start(); err != nil {
		log.Fatalf("启动优雅关闭管理器失败: %s", err.Error())
	}

	// 启动通用API服务器（阻塞当前进程）
	return s.genericAPIServer.Run()
}

// completedExtraConfig 是完善后的ExtraConfig（补全默认值）
type completedExtraConfig struct {
	*ExtraConfig
}

// complete 补全ExtraConfig的默认值（如默认地址）
func (c *ExtraConfig) complete() *completedExtraConfig {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8081" // 若未指定地址，使用默认地址
	}

	return &completedExtraConfig{c}
}

// New 基于完善后的配置创建gRPC服务器实例
func (c *completedExtraConfig) New() (*grpcAPIServer, error) {

	// 关键：生成证书（若不存在）
	if err := c.ServerCert.GenerateCertIfNotExist(); err != nil {
		log.Fatalf("生成/加载TLS证书失败: %s", err.Error())
	}

	// 加载TLS证书，用于gRPC安全连接
	creds, err := credentials.NewServerTLSFromFile(c.ServerCert.CertKey.CertFile, c.ServerCert.CertKey.KeyFile)
	if err != nil {
		log.Fatalf("生成TLS证书失败 %s", err.Error())
	}
	// 配置gRPC服务器选项（最大消息大小、TLS认证）
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize), grpc.Creds(creds)}
	grpcServer := grpc.NewServer(opts...)

	// 初始化MySQL存储客户端
	storeIns, _ := mysql.GetMySQLFactoryOr(c.mysqlOptions)
	// storeIns, _ := etcd.GetEtcdFactoryOr(c.etcdOptions, nil) // 注释：预留Etcd支持
	store.SetClient(storeIns) // 注册存储客户端到全局store

	// 初始化缓存控制器实例（实现gRPC服务接口）
	cacheIns, err := cachev1.GetCacheInsOr(storeIns)
	if err != nil {
		log.Fatalf("获取缓存控制器实例失败: %s", err.Error())
	}

	// 注册缓存gRPC服务到服务器
	pb.RegisterCacheServer(grpcServer, cacheIns)

	// 注册gRPC反射服务（用于调试，可通过工具查看服务定义）
	reflection.Register(grpcServer)

	return &grpcAPIServer{grpcServer, c.Addr}, nil
}

// buildGenericConfig 从配置构建通用API服务器的配置
func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	// 应用通用服务器运行选项（如监听地址、超时设置等）
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	// 应用特性开关配置
	if lastErr = cfg.FeatureOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	// 应用安全服务配置（如HTTPS相关）
	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	// 应用非安全服务配置（如HTTP相关）
	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

// buildExtraConfig 从配置构建gRPC服务器的额外配置
// nolint: unparam 忽略"参数未使用"的 lint 警告
func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		Addr:         fmt.Sprintf("%s:%d", cfg.GRPCOptions.BindAddress, cfg.GRPCOptions.BindPort), // 构建gRPC监听地址
		MaxMsgSize:   cfg.GRPCOptions.MaxMsgSize,                                                  // gRPC最大消息大小
		ServerCert:   cfg.SecureServing.ServerCert,                                                // TLS证书配置
		mysqlOptions: cfg.MySQLOptions,                                                            // MySQL配置
		// etcdOptions:      cfg.EtcdOptions,                                                      // 注释：预留Etcd配置
	}, nil
}

// initRedisStore 初始化Redis连接，并注册关闭回调
func (s *apiServer) initRedisStore() {
	// 创建可取消的上下文（用于控制Redis连接的生命周期）
	ctx, cancel := context.WithCancel(context.Background())
	// 注册Redis连接关闭回调：服务退出时取消上下文，终止Redis连接
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()
		return nil
	}))

	// 构建Redis连接配置（从redisOptions转换）
	config := &storage.Config{
		Host:                  s.redisOptions.Host,
		Port:                  s.redisOptions.Port,
		Addrs:                 s.redisOptions.Addrs,
		MasterName:            s.redisOptions.MasterName,
		Username:              s.redisOptions.Username,
		Password:              s.redisOptions.Password,
		Database:              s.redisOptions.Database,
		MaxIdle:               s.redisOptions.MaxIdle,
		MaxActive:             s.redisOptions.MaxActive,
		Timeout:               s.redisOptions.Timeout,
		EnableCluster:         s.redisOptions.EnableCluster,
		UseSSL:                s.redisOptions.UseSSL,
		SSLInsecureSkipVerify: s.redisOptions.SSLInsecureSkipVerify,
	}

	// 异步连接Redis（避免阻塞服务启动）
	go storage.ConnectToRedis(ctx, config)
}
