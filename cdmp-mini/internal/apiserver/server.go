package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

type apiServer struct {
	genericAPIServer *genericapiserver.GenericAPIServer
}

func createApiServer(cfg *config.Config) (*apiServer, error) {

	storeIns, _ := mysql.GetMySQLFactoryOr(cfg.MySQLOptions)
	store.SetClient(storeIns)

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}
	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}

	server := &apiServer{
		genericAPIServer: genericServer,
	}
	return server, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return nil, lastErr
	}
	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return nil, lastErr
	}
	return
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
