package server

import (
	"net/http"

	"github.com/gin-contrib/pprof"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	ginprometheus "github.com/zsais/go-gin-prometheus"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

type GenericAPIServer struct {
	middlewares []string
	*gin.Engine
	healthz                      bool
	enableMetrics                bool
	enableProfiling              bool
	insecureServer, secureServer *http.Server
}

func initGenericAPIServer(s *GenericAPIServer) {
	//打印gin路由信息
	s.Setup()
	s.InstallMiddlewares()
	s.InstallAPIs()

}

// ）设置路由调试日志
func (s *GenericAPIServer) Setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %-s --> %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

func (s *GenericAPIServer) InstallMiddlewares() {
	s.Use(middleware.RequestID())
	s.Use(middleware.Context())

	for _, m := range s.middlewares {
		mw, ok := middleware.Middlewares[m]
		if !ok {
			log.Warnf("找不到中间件:%s", m)
			continue
		}
		log.Infof("安装中间件%s", m)
		s.Use(mw)
	}

}

func (s *GenericAPIServer) InstallAPIs() {
	if s.healthz {
		s.GET("/healthz", func(c *gin.Context) {
			core.WriteResponse(c, nil, map[string]string{"status": "ok"})
		})
	}
	if s.enableMetrics {
		prometheus := ginprometheus.NewPrometheus("gin")
		prometheus.Use(s.Engine)
	}

	if s.enableProfiling {
		pprof.Register(s.Engine)
	}

	s.GET("/version", func(c *gin.Context) {
		core.WriteResponse(c, nil, version.Get())
	})

}
