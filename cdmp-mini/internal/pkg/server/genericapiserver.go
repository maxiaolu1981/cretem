package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"golang.org/x/sync/errgroup"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	ginprometheus "github.com/zsais/go-gin-prometheus"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

type GenericAPIServer struct {
	middlewares         []string
	InsecureServingInfo *InsecureServingInfo
	healthz             bool
	*gin.Engine
	insecureServer  *http.Server
	enableProfiling bool
	enableMetrics   bool
}

func initGenericAPIServer(s *GenericAPIServer) {
	s.setup()
	s.InstallMiddlewares()
	s.InstallAPIs()
}

// ）设置路由调试日志
func (s *GenericAPIServer) setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %s --> %s(%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
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

func (s *GenericAPIServer) Run() error {
	s.insecureServer = &http.Server{
		Addr:    s.insecureServer.Addr,
		Handler: s,
	}
	var eg errgroup.Group
	eg.Go(func() error {
		log.Infof("开始启动http服务器,地址%s", s.InsecureServingInfo.Address)
		if err := s.insecureServer.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())
			return err
		}
		log.Infof("server on %s stoped", s.InsecureServingInfo.Address)
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.healthz {
		if err := s.ping(ctx); err != nil {
			return err
		}
	}
	if err := eg.Wait(); err != nil {
		log.Fatal(err.Error())
	}
	return nil
}
func (s *GenericAPIServer) ping(ctx context.Context) error {
	url := fmt.Sprintf("http://%s/healthz", s.insecureServer.Addr)
	if strings.Contains(url, "0.0.0.0") {
		url = fmt.Sprintf("http://127.0.0.1:%s/healthz", strings.Split(s.insecureServer.Addr, ":")[1])
	}
	for {
		requ, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(requ)
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Info("路由器已经正确部署.")
			resp.Body.Close()
			return nil
		}
		log.Info("等待服务器路由部署,等待一秒钟")
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			log.Fatal("在指定时间内无法联系服务器")
		default:
		}
	}
}
