package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

type JwtInfo struct {
	Realm      string
	Key        string
	Timeout    time.Duration
	MaxRefresh time.Duration
}

type InsecureServingInfo struct {
	Address string
}

type Config struct {
	InsecureServing *InsecureServingInfo
	Jwt             *JwtInfo
	Mode            string
	EnableMetrics   bool
	EnableProfiling bool
	Healthz         bool
	Middlewares     []string
}

func NewConfig() *Config {
	return &Config{
		Mode:            gin.ReleaseMode,
		EnableMetrics:   true,
		EnableProfiling: true,
		Jwt: &JwtInfo{
			Realm:      "iam jwt",
			Timeout:    1 * time.Hour,
			MaxRefresh: 1 * time.Hour,
		},
		Healthz: true,
	}
}

func (c *Config) Complete() CompleteConfig {
	return CompleteConfig{c}
}

type CompleteConfig struct {
	*Config
}

func (c CompleteConfig) New() (*GenericAPIServer, error) {
	gin.SetMode(c.Mode)
	s := &GenericAPIServer{
		EnableProfiling:     c.EnableProfiling,
		InsecureServingInfo: c.InsecureServing,
		EnableMetrics:       c.EnableMetrics,
		Healthz:             c.Healthz,
		Middlewares:         c.Middlewares,
		Engine:              gin.New(),
	}
	initGenericAPIServer(s)
	return s, nil
}
