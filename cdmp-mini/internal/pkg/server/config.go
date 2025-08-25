package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

type InsecureServingInfo struct {
	BindAddress string
	BindPort    int
}

type jwtInfo struct {
	Realm      string
	Key        string
	Timeout    time.Duration
	MaxRefresh time.Duration
}

type Config struct {
	InsecureServingInfo *InsecureServingInfo
	Mode                string
	EnableProfiling     bool
	EnableMetrics       bool
	Middlewares         []string
	Healthz             bool
	Jwt                 *jwtInfo
}

func NewConfig() *Config {
	return &Config{
		Mode:            gin.ReleaseMode,
		Healthz:         true,
		EnableProfiling: true,
		EnableMetrics:   true,
		Middlewares:     []string{},
		Jwt: &jwtInfo{
			Realm:      "iam jwt",
			Timeout:    1 * time.Second,
			MaxRefresh: 1 * time.Second,
		},
	}
}

type CompleteConfig struct {
	*Config
}

func (c *Config) Complete() *CompleteConfig {
	return &CompleteConfig{c}
}

func (c *CompleteConfig) New() (*GenericAPIServer, error) {
	gin.SetMode(c.Mode)
	genericAPIServer := &GenericAPIServer{
		middlewares:         c.Middlewares,
		healthz:             c.Healthz,
		enableMetrics:       c.EnableMetrics,
		enableProfiling:     c.EnableProfiling,
		insecureServingInfo: c.InsecureServingInfo,
		Engine:              gin.New(),
	}
	err := initGenericAPIServer(genericAPIServer)
	if err != nil {
		return nil, err
	}
	return genericAPIServer, nil
}
