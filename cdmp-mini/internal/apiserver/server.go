package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	genericapiserver "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
)

type apiServer struct {
	genericAPIServer *genericapiserver.InsecureServingInfo
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
}

func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		mySQLOptions: cfg.MySQLOptions,
	}, nil
}
