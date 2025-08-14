package server

import "github.com/gin-gonic/gin"

type InsecureServingInfo struct {
	Address string
}

type Config struct {
	InsecureServing *InsecureServingInfo
	Mode            string
	Middlewares     []string
	Healthz         bool
	EnableProfiling bool
	EnableMetrics   bool
}

func NewConfig() *Config {
	return &Config{
		Healthz:         true,
		Mode:            gin.ReleaseMode,
		Middlewares:     []string{},
		EnableProfiling: true,
		EnableMetrics:   true,
	}
}

func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{c}
}

func (c CompletedConfig) New() (*GenericAPIServer, error) {
	gin.SetMode(c.Mode)
	s := &GenericAPIServer{
		InsecureServingInfo: c.InsecureServing,
		healthz:             c.Healthz,
		enableMetrics:       c.EnableMetrics,
		enableProfiling:     c.EnableProfiling,
		middlewares:         c.Middlewares,
		Engine:              gin.New(),
	}
	initGenericAPIServer(s)
	return s, nil

}

type CompletedConfig struct {
	*Config
}
