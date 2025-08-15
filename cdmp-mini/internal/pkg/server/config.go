package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

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
	Jwt             *JwtInfo
}

func NewConfig() *Config {
	return &Config{
		Healthz:         true,
		Mode:            gin.ReleaseMode,
		Middlewares:     []string{},
		EnableProfiling: true,
		EnableMetrics:   true,
		Jwt: &JwtInfo{
			Realm:      "iam jwt",
			Timeout:    1 * time.Hour,
			MaxRefresh: 1 * time.Hour,
		},
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

// JwtInfo defines jwt fields used to create jwt authentication middleware.
type JwtInfo struct {
	// defaults to "iam jwt"
	Realm string
	// defaults to empty
	Key string
	// defaults to one hour
	Timeout time.Duration
	// defaults to zero
	MaxRefresh time.Duration
}
