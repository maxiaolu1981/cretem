package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/common"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

func InstallMiddlewares(engine *gin.Engine, opts *options.Options) error {
	var stack []gin.HandlerFunc

	// 🔴 最前端日志：在所有中间件执行前打印原始头
	stack = append(stack, func(c *gin.Context) {
		if c.Request.Method == http.MethodDelete && c.Request.URL.Path == "/logout" {
			authHeader := c.GetHeader("Authorization")
			log.Infof("[最前端] 原始Authorization头：[%q]，长度：%d", authHeader, len(authHeader))
		}
		c.Next()
	})

	// 安装通用中间件
	commonMiddlewares := common.GetMiddlewareStack()
	for _, mw := range commonMiddlewares {
		engine.Use(mw)
	}

	// 安装业务中间件（根据配置决定）

	return nil
}
