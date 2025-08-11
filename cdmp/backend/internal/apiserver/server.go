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
package apiserver

import (
	"context"
	"fmt"

	pb "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/config"
	cachev1 "github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/controller/v1/cache"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store/mysql"
	genericoptions "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/options"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/shutdown"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/storage"
)

type apiServer struct {
	gs               *shutdown.GracefulShutdown
	redisOptions     *genericoptions.RedisOptions
	gRPCAPIServer    *grpcAPIServer
	genericAPIServer *genericapiserver.GenericAPIServer
}

type preparedAPIServer struct {
	*apiServer
}

// ExtraConfig defines extra configuration for the iam-apiserver.
type ExtraConfig struct {
	Addr         string
	MaxMsgSize   int
	ServerCert   genericoptions.GeneratableKeyCert
	mysqlOptions *genericoptions.MySQLOptions
	// etcdOptions      *genericoptions.EtcdOptions
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	extraConfig, err := buildExtraConfig(cfg)
	if err != nil {
		return nil, err
	}

	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}
	extraServer, err := extraConfig.complete().New()
	if err != nil {
		return nil, err
	}

	server := &apiServer{
		gs:               gs,
		redisOptions:     cfg.RedisOptions,
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
	}

	return server, nil
}

func (s *apiServer) PrepareRun() preparedAPIServer {
	initRouter(s.genericAPIServer.Engine)

	s.initRedisStore()

	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		mysqlStore, _ := mysql.GetMySQLFactoryOr(nil)
		if mysqlStore != nil {
			_ = mysqlStore.Close()
		}

		s.gRPCAPIServer.Close()
		s.genericAPIServer.Close()

		return nil
	}))

	return preparedAPIServer{s}
}

func (s preparedAPIServer) Run() error {
	go s.gRPCAPIServer.Run()

	// start shutdown managers
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	return s.genericAPIServer.Run()
}

type completedExtraConfig struct {
	*ExtraConfig
}

// Complete fills in any fields not set that are required to have valid data and can be derived from other fields.
func (c *ExtraConfig) complete() *completedExtraConfig {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8081"
	}

	return &completedExtraConfig{c}
}

// New create a grpcAPIServer instance.
func (c *completedExtraConfig) New() (*grpcAPIServer, error) {
	creds, err := credentials.NewServerTLSFromFile(c.ServerCert.CertKey.CertFile, c.ServerCert.CertKey.KeyFile)
	if err != nil {
		log.Fatalf("Failed to generate credentials %s", err.Error())
	}
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize), grpc.Creds(creds)}
	grpcServer := grpc.NewServer(opts...)

	storeIns, _ := mysql.GetMySQLFactoryOr(c.mysqlOptions)
	// storeIns, _ := etcd.GetEtcdFactoryOr(c.etcdOptions, nil)
	store.SetClient(storeIns)
	cacheIns, err := cachev1.GetCacheInsOr(storeIns)
	if err != nil {
		log.Fatalf("Failed to get cache instance: %s", err.Error())
	}

	pb.RegisterCacheServer(grpcServer, cacheIns)

	reflection.Register(grpcServer)

	return &grpcAPIServer{grpcServer, c.Addr}, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.FeatureOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

// nolint: unparam
func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		Addr:         fmt.Sprintf("%s:%d", cfg.GRPCOptions.BindAddress, cfg.GRPCOptions.BindPort),
		MaxMsgSize:   cfg.GRPCOptions.MaxMsgSize,
		ServerCert:   cfg.SecureServing.ServerCert,
		mysqlOptions: cfg.MySQLOptions,
		// etcdOptions:      cfg.EtcdOptions,
	}, nil
}

func (s *apiServer) initRedisStore() {
	ctx, cancel := context.WithCancel(context.Background())
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()

		return nil
	}))

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

	// try to connect to redis
	go storage.ConnectToRedis(ctx, config)
}
