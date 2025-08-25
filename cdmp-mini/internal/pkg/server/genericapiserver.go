package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"golang.org/x/sync/errgroup"
)

type GenericAPIServer struct {
	insecureServingInfo *InsecureServingInfo
	insecureServer      *http.Server
	middlewares         []string
	mode                string
	enableMetrics       bool
	enableProfiling     bool
	healthz             bool
	*gin.Engine
}

func initGenericAPIServer(s *GenericAPIServer) error {
	if s.Engine == nil {
		log.Errorf("Gin引擎没有正确初始化")
		return fmt.Errorf("Gin引擎没有正确初始化")
	}
	s.setup()
	s.installMiddlewares()
	s.installAPIs()
	return nil
}

func (s *GenericAPIServer) setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		log.Infof("%-6s %s ==> %s %d(handers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

func (s *GenericAPIServer) installMiddlewares() {
	s.Use(middleware.RequestID())
	s.Use(middleware.Context())

	for _, mw := range s.middlewares {
		m, ok := middleware.Middlewares[mw]
		if !ok {
			log.Warnf("中间件%s安装失败", m)
			continue
		}
		log.Infof("中间件%s安装成功", m)
		s.Use(m)
	}
}

func (s *GenericAPIServer) installAPIs() {
	if s.healthz {
		s.GET("/healthz", func(ctx *gin.Context) {
			core.WriteResponse(ctx, nil, map[string]string{"status": "ok"})
		})
	}
	if s.enableMetrics {
		prometheus := ginprometheus.NewPrometheus("gin")
		prometheus.Use(s.Engine)
	}
	if s.enableProfiling {
		pprof.Register(s.Engine)
	}
	s.GET("/version", func(ctx *gin.Context) {
		core.WriteResponse(ctx, nil, version.Get())
	})
}

func (s *GenericAPIServer) Run() error {
	address := net.JoinHostPort(s.insecureServingInfo.BindAddress, strconv.Itoa(s.insecureServingInfo.BindPort))

	s.insecureServer = &http.Server{
		Handler: s.Engine,
		Addr:    address,
	}
	var eg errgroup.Group

	eg.Go(func() error {
		log.Infof("开始在%s运行api服务器", address)
		if err := s.insecureServer.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err.Error())
			return err
		}
		log.Infof("服务在%s停止", address)
		return nil
	})

	if s.healthz {
		ctx, canel := context.WithTimeout(context.Background(), 10*time.Second)
		defer canel()
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
	address := net.JoinHostPort(s.insecureServingInfo.BindAddress, strconv.Itoa(s.insecureServingInfo.BindPort))
	url := fmt.Sprintf("http://%s/healthz", address)
	if strings.Contains(url, "0.0.0.0") {
		url = fmt.Sprintf("http//127.0.0.1:%s", strings.Split(address, ":")[1])
	}
	for {
		requ, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil
		}
		if resp, err := http.DefaultClient.Do(requ); err == nil && resp.StatusCode == http.StatusOK {
			log.Info("路由加载成功....")
			return nil
		}
		log.Info("路由服务正在加载,1秒后再试")
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			log.Fatal("路由加载失败")
		default:
		}

	}
}
