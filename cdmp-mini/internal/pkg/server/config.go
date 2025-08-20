package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	Mode                string
	InsecureServingInfo *InsecureServingInfo
	Healthz             bool
	EnableProfiling     bool
	EnableMetrics       bool
	Middlewares         []string
	Jwt                 *JwtInfo
}

func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{c}
}

type JwtInfo struct {
	Realm      string
	Key        string
	Timeout    time.Duration
	MaxRefresh time.Duration
}

func NewConfig() *Config {
	return &Config{
		Mode:            gin.ReleaseMode,
		Middlewares:     []string{},
		Healthz:         true,
		EnableProfiling: true,
		EnableMetrics:   true,
		Jwt: &JwtInfo{
			Realm:      "iam-server",
			Timeout:    1 * time.Hour,
			MaxRefresh: 1 * time.Hour,
		},
	}
}

type CompletedConfig struct {
	*Config
}

func (c CompletedConfig) New() (*GenericAPIServer, error) {
	gin.SetMode(c.Mode)
	s := &GenericAPIServer{
		InsecureServingInfo: c.InsecureServingInfo,
		healthz:             c.Healthz,
		middlewares:         c.Middlewares,
		enableProfiling:     c.EnableProfiling,
		enableMetrics:       c.EnableMetrics,
		Engine:              gin.New(),
	}
	initGenericAPIServer(s)
	return s, nil
}

type InsecureServingInfo struct {
	Address string
}
