package common

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	xcode "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 增加并发请求计数
		metrics.HTTPRequestsInProgress.WithLabelValues(
			getFullPath(c),
			c.Request.Method,
		).Inc()

		defer func() {
			// 减少并发请求计数
			metrics.HTTPRequestsInProgress.WithLabelValues(
				getFullPath(c),
				c.Request.Method,
			).Dec()
		}()

		metrics.HTTPMiddlewareStart()

		var requestSize int64
		if c.Request.Body != nil {
			requestSize = c.Request.ContentLength
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		responseSize := c.Writer.Size()
		if responseSize < 0 {
			responseSize = 0
		}

		// 获取业务维度信息
		userID, tenantID := getBusinessDimensions(c)
		errorCode := getErrorCode(c)
		errorType := getErrorType(c, c.Errors.Last())

		// 记录HTTP请求总数
		metrics.HTTPRequestsTotal.WithLabelValues(
			getFullPath(c),
			c.Request.Method,
			status,
			errorType,
			userID,
			tenantID,
			c.ClientIP(),
			c.Request.UserAgent(),
			c.Request.Host,
		).Inc()

		// 记录HTTP请求延迟分布
		metrics.HTTPRequestDuration.WithLabelValues(
			getFullPath(c),
			c.Request.Method,
			status,
			errorType,
		).Observe(duration)

		// 记录详细的HTTP请求指标
		// 记录详细的HTTP请求指标
		metrics.RecordHTTPRequest(
			getFullPath(c),
			c.Request.Method,
			status,
			duration,
			requestSize,
			int64(responseSize),
			c.ClientIP(),
			c.Request.UserAgent(),
			c.Request.Host,
			strconv.Itoa(errorCode),
			errorType,
			userID,
			tenantID,
		)

		// 记录缓存命中情况（如果存在）
		if cacheHit, exists := c.Get("cache_hit"); exists {
			metrics.CacheRequests.WithLabelValues(
				getFullPath(c),
				fmt.Sprintf("%v", cacheHit),
				userID,
				tenantID,
			).Inc()
		}

		// 记录错误指标
		if c.Writer.Status() >= 400 {
			// 记录500错误详情
			if c.Writer.Status() == 500 {
				log.Errorf("500系统错误 - 路径: %s, 方法: %s, 错误码: %d, 错误类型: %s, 用户: %s, 租户: %s, 错误: %v",
					c.Request.URL.Path, c.Request.Method, errorCode, errorType, userID, tenantID, c.Errors.Last())
			}

			metrics.HTTPErrors.WithLabelValues(
				c.Request.Method,
				getFullPath(c),
				status,
				errorType,
				strconv.Itoa(errorCode),
				userID,
				tenantID,
			).Inc()
		}

		// 记录慢请求（超过1秒）
		if duration > 1.0 {
			log.Warnf("慢请求告警 - 路径: %s, 方法: %s, 延迟: %.3fs, 状态: %s, 用户: %s, 租户: %s",
				getFullPath(c), c.Request.Method, duration, status, userID, tenantID)

			metrics.SlowHTTPRequests.WithLabelValues(
				getFullPath(c),
				c.Request.Method,
				status,
				errorType,
			).Inc()
		}

		metrics.HTTPMiddlewareEnd()
	}
}

// getFullPath 安全地获取完整路径
func getFullPath(c *gin.Context) string {
	path := c.FullPath()
	// 如果FullPath为空，说明路由未匹配，使用固定值
	if path == "" {
		path = "unmatched_route"
	}
	return path
}

// getBusinessDimensions 获取业务维度信息
func getBusinessDimensions(c *gin.Context) (string, string) {
	userID := "unknown"
	tenantID := "unknown"

	// 从JWT token或上下文中获取用户信息
	if userVal, exists := c.Get("user_id"); exists {
		userID = fmt.Sprintf("%v", userVal)
	}
	if tenantVal, exists := c.Get("tenant_id"); exists {
		tenantID = fmt.Sprintf("%v", tenantVal)
	}

	return userID, tenantID
}

// getErrorCode 获取错误码（完善版本）
func getErrorCode(c *gin.Context) int {
	// 优先从上下文获取
	if codeVal, exists := c.Get("error_code"); exists {
		if code, ok := codeVal.(int); ok {
			return code
		}
	}

	// 从错误中解析
	if err := c.Errors.Last(); err != nil {
		if coder := errors.ParseCoderByErr(err); coder != nil {
			return coder.Code()
		}
	}

	// 根据HTTP状态码返回默认错误码
	status := c.Writer.Status()
	switch {
	case status >= 400 && status < 500:
		return xcode.ErrInvalidParameter
	case status >= 500:
		return xcode.ErrInternalServer
	default:
		return 0 // 成功请求
	}
}

func getErrorTypeFromError(err error) string {
	if err == nil {
		return "unknown_error"
	}

	// 安全地获取类型信息
	errType := fmt.Sprintf("%T", err)
	lowerType := strings.ToLower(errType)

	// 安全地获取错误信息（防panic）
	errMsg, _ := safelyGetErrorMessage(err)
	lowerMsg := strings.ToLower(errMsg)

	// 结合类型和消息进行判断
	switch {
	case strings.Contains(lowerType, "timeout") || strings.Contains(lowerMsg, "timeout"):
		return "timeout"
	case strings.Contains(lowerType, "database") || strings.Contains(lowerMsg, "database"):
		return "database_error"
	case strings.Contains(lowerType, "redis") || strings.Contains(lowerMsg, "redis"):
		return "redis_error"
	case strings.Contains(lowerType, "kafka") || strings.Contains(lowerMsg, "kafka"):
		return "kafka_error"
	case strings.Contains(lowerMsg, "connection"), strings.Contains(lowerMsg, "connect"):
		return "connection_error"
	case strings.Contains(lowerMsg, "auth"), strings.Contains(lowerMsg, "token"):
		return "authentication_error"
	case strings.Contains(lowerMsg, "validation"), strings.Contains(lowerMsg, "invalid"):
		return "validation_error"
	case strings.Contains(lowerMsg, "not found"), strings.Contains(lowerMsg, "not exist"):
		return "not_found"
	default:
		return "unknown_error"
	}
}

// safelyGetErrorMessage 安全地获取错误信息
func safelyGetErrorMessage(err error) (string, bool) {
	defer func() {
		if recover() != nil {
			// 忽略所有panic
		}
	}()

	// 检查err是否为nil指针
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return "nil_error", true
	}

	// 尝试调用Error()方法
	return err.Error(), true
}

func getErrorType(c *gin.Context, err error) string {
	// 首先尝试从gin上下文获取错误码
	if errorCode, exists := c.Get("error_code"); exists {
		if code, ok := errorCode.(int); ok {
			return getErrorTypeFromCode(code)
		}
	}

	// 直接使用框架的 ParseCoderByErr 函数解析错误
	if coder := errors.ParseCoderByErr(err); coder != nil {
		return getErrorTypeFromCode(coder.Code())
	}

	// 对于非框架错误，使用基于错误信息的类型判断
	return getErrorTypeFromError(err)
}

// getErrorTypeFromCode 根据错误码判断错误类型
func getErrorTypeFromCode(errorCode int) string {
	// 使用框架的 ParseCoderByCode 获取错误信息
	coder := errors.ParseCoderByCode(errorCode)
	if coder == nil {
		return "unknown_error"
	}

	// 可以根据错误码范围或特定错误码返回对应的错误类型
	code := coder.Code()

	// 使用框架的错误码常量进行比较
	switch {
	case code == xcode.ErrUnknown:
		return "unknown_error"
	case code == xcode.ErrBind || code == xcode.ErrInvalidParameter:
		return "bad_request"
	case code == xcode.ErrValidation:
		return "validation_error"
	case code == xcode.ErrPageNotFound || code == xcode.ErrUserNotFound || code == xcode.ErrSecretNotFound || code == xcode.ErrPolicyNotFound:
		return "not_found"
	case code == xcode.ErrMethodNotAllowed:
		return "method_not_allowed"
	case code == xcode.ErrContextCanceled:
		return "request_timeout"

	// 数据库相关错误
	case code == xcode.ErrDatabase || code == xcode.ErrDatabaseTimeout || code == xcode.ErrDatabaseDeadlock:
		return "database_error"

	// 认证授权相关错误
	case code >= 100200 && code < 100300:
		return "authentication_error"

	// 编码解码错误
	case code >= 100300 && code < 100400:
		return "encoding_error"

	// Kafka/Redis错误
	case code == xcode.ErrKafkaFailed || code == xcode.ErrRedisFailed:
		if code == xcode.ErrKafkaFailed {
			return "kafka_error"
		}
		return "redis_error"

	// 业务错误
	case code >= 110000 && code < 120000:
		return "business_error"

	default:
		// 根据错误码范围判断大类
		if code >= 100000 && code < 101000 {
			return "common_error"
		} else if code >= 110000 && code < 120000 {
			return "business_error"
		}
		return "unknown_error"
	}
}
