package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
)

type apiserver struct {
	genericapiserver *genericapiserver.GenericAPIServer
}
type prepareAPIServer struct {
	*apiserver
}

func (a *apiserver) prepareRun() prepareAPIServer {
	initRouter(a.genericapiserver.Engine)
	return prepareAPIServer{a}
}

func (p prepareAPIServer) Run() error {
	return p.genericapiserver.Run()
}

func createAPIServer(cfg *config.Config) (*apiserver, error) {
	storeIns, err := mysql.GetMySQLFactoryOr(cfg.MySQLOptions)
	if err != nil {
		return nil, err
	}
	store.SetClient(storeIns)

	genericapiserverConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}
	genericapiserver, err := genericapiserverConfig.Complete().New()
	if err != nil {
		return nil, err
	}
	server := &apiserver{
		genericapiserver: genericapiserver,
	}
	return server, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lasterr error) {
	genericConfig = genericapiserver.NewConfig()
	if lasterr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lasterr != nil {
		return
	}
	if lasterr = cfg.InsecureServing.ApplyTo(genericConfig); lasterr != nil {
		return
	}
	return
}
