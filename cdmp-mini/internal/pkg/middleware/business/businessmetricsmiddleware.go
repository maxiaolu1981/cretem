package business

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
)

// BusinessMetricsMiddleware 业务监控中间件
func BusinessMetricsMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 获取操作标识（使用路由路径或自定义逻辑）
		operation := getOperationName(c)
		source := "http"

		// 开始业务操作监控
		metrics.BusinessInProgress.WithLabelValues(serviceName, operation).Inc()
		metrics.BusinessOperationsTotal.WithLabelValues(serviceName, operation, source).Inc()

		// 处理请求
		c.Next()

		// 计算处理时长
		duration := time.Since(start).Seconds()

		// 记录处理时长
		metrics.BusinessProcessingTime.WithLabelValues(serviceName, operation).Observe(duration)
		metrics.BusinessThroughputStats.WithLabelValues(serviceName, operation).Observe(duration)

		// 判断请求结果
		status := c.Writer.Status()
		var err error
		if status >= 400 {
			// 如果有具体的错误信息，使用最后一个错误
			if lastErr := c.Errors.Last(); lastErr != nil {
				err = lastErr
			} else {
				err = fmt.Errorf("http_status_%d", status)
			}
		}

		// 记录成功/失败
		if err != nil {
			errorType := metrics.GetBusinessErrorType(err)
			metrics.BusinessFailures.WithLabelValues(serviceName, operation, errorType).Inc()
		} else {
			metrics.BusinessSuccess.WithLabelValues(serviceName, operation, "success").Inc()
		}

		// 减少处理中计数
		metrics.BusinessInProgress.WithLabelValues(serviceName, operation).Dec()
	}
}

// getOperationName 获取操作名称（可自定义逻辑）
func getOperationName(c *gin.Context) string {
	// 方案1：使用路由路径
	if fullPath := c.FullPath(); fullPath != "" {
		return normalizePath(fullPath)
	}

	// 方案2：使用请求路径
	return normalizePath(c.Request.URL.Path)
}

// normalizePath 规范化路径，避免因为参数导致指标基数过大
func normalizePath(path string) string {
	// 将ID等参数替换为占位符
	path = strings.TrimSuffix(path, "/")

	// 替换UUID/ID参数
	if strings.Contains(path, "/") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if isIDParam(part) {
				parts[i] = ":id"
			}
		}
		path = strings.Join(parts, "/")
	}

	return path
}

// isIDParam 判断是否为ID参数
func isIDParam(s string) bool {
	if len(s) == 0 {
		return false
	}

	// UUID格式
	if matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, s); matched {
		return true
	}

	// 数字ID
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		return true
	}

	return false
}
