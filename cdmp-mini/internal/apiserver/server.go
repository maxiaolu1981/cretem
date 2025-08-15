package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

type apiServer struct {
	genericAPIServer *genericapiserver.GenericAPIServer
	gRPCAPIServer    *grpcAPIServer
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
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
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
	}
	return server, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {

	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	if lastErr = cfg.FeatureOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	return
}

type ExtraConfig struct {
	mySQLOptions *genericoptions.MySQLOptions
	Addr         string
}

func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		mySQLOptions: cfg.MySQLOptions,
		Addr:         "127.0.0.1",
	}, nil
}

func (c *ExtraConfig) complete() *completedExtraConfig {
	return &completedExtraConfig{
		ExtraConfig: c,
	}
}

type completedExtraConfig struct {
	*ExtraConfig
}

func (c *completedExtraConfig) New() (*grpcAPIServer, error) {
	storeIns, _ := mysql.GetMySQLFactoryOr(c.mySQLOptions)
	store.SetClient(storeIns)
	return &grpcAPIServer{}, nil

}

type preparedAPIServer struct {
	*apiServer
}

func (s *apiServer) PrepareRun() preparedAPIServer {
	initRouter(s.genericAPIServer.Engine)
	return preparedAPIServer{s}
}

func (s preparedAPIServer) Run() error {
	log.Info("服务器就绪...................")
	return s.genericAPIServer.Run()
}
