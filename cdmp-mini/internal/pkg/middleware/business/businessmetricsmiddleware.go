package business

import (
	"regexp"

	"strings"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
)

// BusinessMetricsMiddleware 业务监控中间件
func BusinessMetricsMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 识别业务操作
		operation := getOperationFromContext(c)

		// 使用现有的StartBusinessOperation开始监控
		timer := metrics.StartBusinessOperation(serviceName, operation, "http")

		// 处理请求
		c.Next()

		// 判断错误并结束监控
		var err error
		if c.Writer.Status() >= 400 {
			if lastErr := c.Errors.Last(); lastErr != nil {
				err = lastErr
			}
		}

		// 使用现有的EndBusinessOperation结束监控
		timer.EndBusinessOperation(err)
	}
}

// 从上下文获取操作名称
func getOperationFromContext(c *gin.Context) string {
	// 使用方法+路径作为操作标识
	method := c.Request.Method
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}

	// 简化的操作名称生成
	return method + "_" + normalizePath(path)
}

// 路径规范化（避免指标基数爆炸）
func normalizePath(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if isIDParam(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isIDParam(s string) bool {
	// 简单的ID参数检测
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^[0-9a-f-]{8,}$`, s); matched {
		return true
	}
	return false
}

// UserServiceMiddleware 用户服务专用中间件
func UserServiceMiddleware() gin.HandlerFunc {
    return BusinessMetricsMiddleware("user_service")
}

// ProductServiceMiddleware 产品服务专用中间件
func ProductServiceMiddleware() gin.HandlerFunc {
    return BusinessMetricsMiddleware("product_service")
}

// APIServiceMiddleware API服务中间件
func APIServiceMiddleware() gin.HandlerFunc {
    return BusinessMetricsMiddleware("api_service")
}