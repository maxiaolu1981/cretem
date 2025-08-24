package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	ginprometheus "github.com/zsais/go-gin-prometheus"

	"golang.org/x/sync/errgroup"
)

type GenericAPIServer struct {
	InsecureServingInfo *InsecureServingInfo
	Jwt                 *JwtInfo
	*gin.Engine
	InsecureServer  *http.Server
	Mode            string
	EnableProfiling bool
	EnableMetrics   bool
	Healthz         bool
	Middlewares     []string
}

func initGenericAPIServer(s *GenericAPIServer) {
	s.Setup()
	s.InstallMiddles()
	s.InstallAPIs()
}

func (s *GenericAPIServer) Setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %-s==> %s %d(handers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

func (s *GenericAPIServer) InstallMiddles() {
	s.Use(middleware.RequestID())
	s.Use(middleware.Context())

	for _, mw := range s.Middlewares {
		m, ok := middleware.Middlewares[mw]
		if !ok {
			log.Warnf("安装中间件失败.%s", m)
			continue
		}
		log.Infof("安装中间件成功%s", m)
		s.Use(m)
	}
}

func (s *GenericAPIServer) InstallAPIs() {
	if s.Healthz {
		s.GET("/healthz", func(ctx *gin.Context) {
			core.WriteResponse(ctx, nil, map[string]string{"status": "ok"})
		})
	}
	if s.EnableProfiling {
		prometheus := ginprometheus.NewPrometheus("gin")
		prometheus.Use(s.Engine)
	}
	if s.EnableMetrics {
		pprof.Register(s)
	}
	s.GET("/version", func(ctx *gin.Context) {
		core.WriteResponse(ctx, nil, version.Get())
	})
}

func (s *GenericAPIServer) Run() error {
	s.InsecureServer = &http.Server{
		Handler:        s,
		Addr:           s.InsecureServingInfo.Address,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	var eg errgroup.Group
	eg.Go(func() error {
		log.Infof("开始启动api服务在端口:%s", s.InsecureServingInfo.Address)
		if err := s.InsecureServer.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("api服务在%s启动失败", err.Error())
			return err
		}
		log.Infof("停止服务器在%s", s.InsecureServingInfo.Address)
		return nil
	})
	ctx, canel := context.WithTimeout(context.Background(), 10*time.Second)
	defer canel()
	if s.Healthz {
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
	url := fmt.Sprintf("http://%s/healthz", s.InsecureServer.Addr)
	if strings.Contains(url, "0.0.0.0") {
		url = fmt.Sprintf("http://1271.0.0.1:%s/healthz", strings.Split(s.InsecureServer.Addr, ":")[1])
	}
	for {
		requ, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(requ)
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Info("路由已经正确部署")
			resp.Body.Close()
			return nil
		}
		log.Info("等待路由部署,1秒钟重试")
		time.Sleep(1 * time.Second)

		select {
		case <-ctx.Done():
			log.Fatal("不能连接到服务器")
		default:
		}

	}

}
