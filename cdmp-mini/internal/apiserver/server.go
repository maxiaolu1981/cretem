package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
)

type apiServer struct {
	genericapiserver *genericapiserver.GenericAPIServer
}

type preparedAPIServer struct {
	*apiServer
}

func (a *apiServer) PrepareRun() preparedAPIServer {
	initRouter(a.genericapiserver.Engine)
	return preparedAPIServer{a}
}

func (p preparedAPIServer) Run() error {
	return p.genericapiserver.Run()
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
	storeIns, err := mysql.GetMySQLFactoryOr(cfg.MySQLOptions)
	if err != nil {
		return nil, err
	}
	store.SetClient(storeIns)

	genericServerConig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}
	genericAPIServer, err := genericServerConig.Complete().New()
	if err != nil {
		return nil, err
	}
	apiServer := &apiServer{
		genericapiserver: genericAPIServer,
	}
	return apiServer, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}
	return

}
