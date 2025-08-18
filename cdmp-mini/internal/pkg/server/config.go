package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

type config struct {
	Mode            string
	Middlewares     []string
	Healthz         bool
	EnableProfiling bool
	EnableMetrics   bool
	Jwt             *JwtInfo
}

func NewConfig() *config {
	return &config{
		Mode:            gin.ReleaseMode,
		Middlewares:     []string{},
		Healthz:         true,
		EnableProfiling: true,
		EnableMetrics:   true,
		Jwt: &JwtInfo{
			Realm:      "iam jwt",
			Timeout:    1 * time.Hour,
			MaxRefresh: 1 * time.Hour,
		},
	}
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
