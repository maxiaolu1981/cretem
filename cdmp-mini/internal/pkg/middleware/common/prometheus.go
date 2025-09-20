package common

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
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

		// 记录HTTP请求指标
		metrics.RecordHTTPRequest(
			getFullPath(c),
			c.Request.Method,
			status,
			duration,
			requestSize,
			int64(responseSize),
		)

		// 记录错误指标
		if c.Writer.Status() >= 400 {
			// 获取错误码（优先从上下文）
			var errorCode int
			if codeVal, exists := c.Get("error_code"); exists {
				if code, ok := codeVal.(int); ok {
					errorCode = code
				}
			}

			errorType := getErrorType(c, c.Writer.Status(), c.Errors.Last())

			// 记录500错误详情
			if c.Writer.Status() == 500 {
				log.Errorf("500系统错误 - 路径: %s, 方法: %s, 错误码: %d, 错误: %v",
					c.Request.URL.Path, c.Request.Method, errorCode, c.Errors.Last())
			}

			metrics.HTTPErrors.WithLabelValues(
				c.Request.Method,
				getFullPath(c),
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
	if path == "" {
		path = c.Request.URL.Path
	}
	return path
}

func getErrorType(c *gin.Context, statusCode int, err error) string {
	// 首先尝试从gin上下文获取错误码
	if errorCode, exists := c.Get("error_code"); exists {
		if code, ok := errorCode.(int); ok {
			return getErrorTypeFromCode(code)
		}
	}

	// 然后尝试从error对象中提取错误码
	if coder := errors.ParseCoderByErr(err); coder != nil {
		return getErrorTypeFromCode(coder.Code())
	}

	// 最后才回退到类型字符串判断
	return getErrorTypeFromError(err)
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

// getErrorTypeFromTypeString 根据类型字符串判断错误类型
func getErrorTypeFromTypeString(typeString string) string {
	lowerType := strings.ToLower(typeString)

	switch {
	case strings.Contains(lowerType, "timeout"):
		return "timeout"
	case strings.Contains(lowerType, "database"), strings.Contains(lowerType, "sql"), strings.Contains(lowerType, "mysql"), strings.Contains(lowerType, "gorm"):
		return "database_error"
	case strings.Contains(lowerType, "connection"), strings.Contains(lowerType, "network"):
		return "connection_error"
	case strings.Contains(lowerType, "auth"), strings.Contains(lowerType, "token"), strings.Contains(lowerType, "jwt"):
		return "authentication_error"
	case strings.Contains(lowerType, "json"), strings.Contains(lowerType, "yaml"):
		return "encoding_error"
	case strings.Contains(lowerType, "validation"):
		return "validation_error"
	case strings.Contains(lowerType, "notfound"):
		return "not_found"
	case strings.Contains(lowerType, "kafka"):
		return "kafka_error"
	case strings.Contains(lowerType, "redis"):
		return "redis_error"
	case strings.Contains(lowerType, "panic"):
		return "panic"
	default:
		return "unknown_error"
	}
}

// getErrorTypeFromErrorMessage 根据错误消息判断错误类型
func getErrorTypeFromErrorMessage(errorMsg string) string {
	lowerMsg := strings.ToLower(errorMsg)

	switch {
	case strings.Contains(lowerMsg, "timeout"), strings.Contains(lowerMsg, "deadline"):
		return "timeout"
	case strings.Contains(lowerMsg, "database"), strings.Contains(lowerMsg, "sql"), strings.Contains(lowerMsg, "table"), strings.Contains(lowerMsg, "column"):
		if strings.Contains(lowerMsg, "deadlock") {
			return "database_deadlock"
		}
		if strings.Contains(lowerMsg, "timeout") {
			return "database_timeout"
		}
		if strings.Contains(lowerMsg, "duplicate") {
			return "database_duplicate"
		}
		return "database_error"
	case strings.Contains(lowerMsg, "connection"), strings.Contains(lowerMsg, "connect"), strings.Contains(lowerMsg, "network"):
		return "connection_error"
	case strings.Contains(lowerMsg, "auth"), strings.Contains(lowerMsg, "token"), strings.Contains(lowerMsg, "password"), strings.Contains(lowerMsg, "login"):
		if strings.Contains(lowerMsg, "expired") {
			return "token_expired"
		}
		if strings.Contains(lowerMsg, "invalid") {
			return "token_invalid"
		}
		if strings.Contains(lowerMsg, "permission") {
			return "permission_denied"
		}
		return "authentication_error"
	case strings.Contains(lowerMsg, "validation"), strings.Contains(lowerMsg, "invalid"), strings.Contains(lowerMsg, "validate"):
		return "validation_error"
	case strings.Contains(lowerMsg, "not found"), strings.Contains(lowerMsg, "not exist"), strings.Contains(lowerMsg, "no such"):
		return "not_found"
	case strings.Contains(lowerMsg, "json"), strings.Contains(lowerMsg, "yaml"), strings.Contains(lowerMsg, "encode"), strings.Contains(lowerMsg, "decode"):
		return "encoding_error"
	case strings.Contains(lowerMsg, "kafka"), strings.Contains(lowerMsg, "producer"), strings.Contains(lowerMsg, "consumer"):
		return "kafka_error"
	case strings.Contains(lowerMsg, "redis"):
		return "redis_error"
	case strings.Contains(lowerMsg, "panic"):
		return "panic"
	case strings.Contains(lowerMsg, "out of memory"), strings.Contains(lowerMsg, "oom"):
		return "out_of_memory"
	case strings.Contains(lowerMsg, "rate limit"), strings.Contains(lowerMsg, "ratelimit"):
		return "rate_limit"
	default:
		return "unknown_error"
	}
}

// getErrorTypeFromStatusCode 根据HTTP状态码判断错误类型
func getErrorTypeFromStatusCode(statusCode int) string {
	switch statusCode {
	case 400:
		return "bad_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 405:
		return "method_not_allowed"
	case 408:
		return "request_timeout"
	case 409:
		return "conflict"
	case 415:
		return "unsupported_media_type"
	case 422:
		return "validation_error"
	case 429:
		return "rate_limit"
	case 500:
		return "internal_server_error"
	case 502:
		return "bad_gateway"
	case 503:
		return "service_unavailable"
	case 504:
		return "gateway_timeout"
	default:
		if statusCode >= 400 && statusCode < 500 {
			return "client_error"
		} else if statusCode >= 500 {
			return "server_error"
		}
		return "unknown"
	}
}

// getErrorTypeFromCode 根据错误码判断错误类型
func getErrorTypeFromCode(errorCode int) string {
	switch errorCode {
	// 通用错误 (1000xx)
	case 100002: // ErrUnknown
		return "unknown_error"
	case 100003: // ErrBind
		return "bad_request"
	case 100004: // ErrValidation
		return "validation_error"
	case 100005: // ErrPageNotFound
		return "not_found"
	case 100006: // ErrMethodNotAllowed
		return "method_not_allowed"
	case 100007: // ErrUnsupportedMediaType
		return "unsupported_media_type"
	case 100008: // ErrContextCanceled
		return "request_timeout"

	// 数据库错误 (1001xx)
	case 100101: // ErrDatabase
		return "database_error"
	case 100102: // ErrDatabaseTimeout
		return "database_timeout"
	case 100103: // ErrDatabaseDeadlock
		return "database_deadlock"

	// 认证授权错误 (1002xx)
	case 100201: // ErrEncrypt
		return "encryption_error"
	case 100202: // ErrSignatureInvalid
		return "signature_invalid"
	case 100203: // ErrExpired
		return "token_expired"
	case 100204: // ErrInvalidAuthHeader
		return "invalid_auth_header"
	case 100205: // ErrMissingHeader
		return "missing_auth_header"
	case 100206: // ErrPasswordIncorrect
		return "password_incorrect"
	case 100207: // ErrPermissionDenied
		return "permission_denied"
	case 100208: // ErrTokenInvalid
		return "token_invalid"
	case 100209: // ErrBase64DecodeFail
		return "base64_decode_error"
	case 100210: // ErrInvalidBasicPayload
		return "invalid_basic_payload"
	case 100211: // ErrRespCodeRTRevoked
		return "token_revoked"
	case 100212: // ErrTokenMismatch
		return "token_mismatch"

	// 编码解码错误 (1003xx)
	case 100301: // ErrEncodingFailed
		return "encoding_error"
	case 100302: // ErrDecodingFailed
		return "decoding_error"
	case 100303: // ErrInvalidJSON
		return "invalid_json"
	case 100304: // ErrEncodingJSON
		return "json_encoding_error"
	case 100305: // ErrDecodingJSON
		return "json_decoding_error"
	case 100306: // ErrInvalidYaml
		return "invalid_yaml"
	case 100307: // ErrEncodingYaml
		return "yaml_encoding_error"
	case 100308: // ErrDecodingYaml
		return "yaml_decoding_error"

	// Kafka错误 (1004xx)
	case 100401: // ErrKafkaSendFailed
		return "kafka_error"
	case 100402:
		return "redis_error"

	// 用户模块错误 (1100xx)
	case 110001: // ErrUserNotFound
		return "user_not_found"
	case 110002: // ErrUserAlreadyExist
		return "user_already_exists"
	case 110003: // ErrUnauthorized
		return "unauthorized"
	case 110004: // ErrInvalidParameter
		return "invalid_parameter"
	case 110005: // ErrInternal
		return "user_internal_error"
	case 110006: // ErrResourceConflict
		return "resource_conflict"
	case 110007: // ErrInternalServer
		return "internal_server_error"

	// 密钥模块错误 (1101xx)
	case 110101: // ErrReachMaxCount
		return "max_limit_reached"
	case 110102: // ErrSecretNotFound
		return "secret_not_found"

	// 策略模块错误 (1102xx)
	case 110201: // ErrPolicyNotFound
		return "policy_not_found"

	default:
		// 根据错误码范围判断大类
		if errorCode >= 100000 && errorCode < 101000 {
			return "common_error"
		} else if errorCode >= 110000 && errorCode < 111000 {
			return "business_error"
		} else if errorCode >= 120000 && errorCode < 121000 {
			return "service_error"
		}
		return "unknown_error"
	}
}
