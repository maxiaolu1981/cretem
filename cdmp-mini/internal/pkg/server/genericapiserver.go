package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"

	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

type GenericAPIServer struct {
	middlewares         []string
	InsecureServingInfo *InsecureServingInfo
	*gin.Engine
	healthz                      bool
	enableMetrics                bool
	enableProfiling              bool
	insecureServer, secureServer *http.Server
}

func initGenericAPIServer(s *GenericAPIServer) {
	//打印gin路由信息
	s.Setup()

}

// ）设置路由调试日志
func (s *GenericAPIServer) Setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %-s --> %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

func (s *GenericAPIServer) InstallMiddlewares() {
	s.Use(middleware.RequestID())
	s.Use(middleware.)
}
