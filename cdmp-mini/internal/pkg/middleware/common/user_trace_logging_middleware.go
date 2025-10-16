package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	apiv1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

// UserTraceConfig controls user trace logging behaviour.
type UserTraceConfig struct {
	Enabled      bool
	ServiceName  string
	Env          string
	PathPrefixes []string
}

// UserTraceLoggingMiddleware emits JSON-formatted spans for user-related APIs.
func UserTraceLoggingMiddleware(cfg UserTraceConfig) gin.HandlerFunc {
	if len(cfg.PathPrefixes) == 0 {
		cfg.PathPrefixes = []string{"/v1/users"}
	}
	hostname, _ := os.Hostname()
	buildInfo := version.Get()

	return func(c *gin.Context) {
		if c == nil {
			return
		}

		if !cfg.Enabled {
			c.Next()
			return
		}

		start := time.Now()
		requestID := GetRequestIDFromContext(c)
		if requestID == "" {
			requestID = GetRequestIDFromHeaders(c)
		}
		if requestID == "" {
			requestID = uuid.Must(uuid.NewV4()).String()
			c.Set(XRequestIDKey, requestID)
			c.Request.Header.Set(XRequestIDKey, requestID)
		}
		spanID := uuid.Must(uuid.NewV4()).String()

		c.Next()

		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}
		if !matchesPrefix(fullPath, cfg.PathPrefixes) {
			return
		}

		end := time.Now()
		durationMs := float64(end.Sub(start)) / float64(time.Millisecond)
		startMs := start.UnixNano() / int64(time.Millisecond)
		endMs := end.UnixNano() / int64(time.Millisecond)

		status := "success"
		if c.Writer.Status() >= http.StatusBadRequest {
			status = "error"
		}

		var errMsg interface{}
		if len(c.Errors) > 0 {
			errMsg = c.Errors.String()
		}

		operation := c.Request.Method + " " + fullPath
		payload := map[string]interface{}{
			"trace": map[string]interface{}{
				"trace_id":       requestID,
				"root_operation": operation,
				"start_time":     startMs,
			},
			"span": map[string]interface{}{
				"operation":   operation,
				"id":          spanID,
				"parent_id":   requestID,
				"service":     nonEmpty(cfg.ServiceName, "iam-apiserver"),
				"start_time":  startMs,
				"end_time":    endMs,
				"duration_ms": durationMs,
				"status":      status,
				"error":       errMsg,
				"component":   "user-api",
			},
			"context": map[string]interface{}{
				"user_id":    extractTraceUserID(c),
				"order_id":   extractTraceOrderID(c),
				"client_ip":  c.ClientIP(),
				"request_id": requestID,
			},
			"resource": map[string]interface{}{
				"host":    hostname,
				"env":     cfg.Env,
				"version": buildInfo.GitVersion,
			},
		}

		data, err := json.Marshal(payload)
		if err != nil {
			log.Debugw("user trace logging marshal failed", "error", err, "operation", operation)
			return
		}
		log.Debug(string(data))
	}
}

func matchesPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func extractTraceUserID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if name := c.GetString("username"); name != "" {
		return name
	}
	if current, exists := c.Get("current_user"); exists {
		switch v := current.(type) {
		case *apiv1.User:
			if v != nil {
				return v.Name
			}
		case interface{ GetName() string }:
			return v.GetName()
		case fmt.Stringer:
			return v.String()
		case string:
			return v
		default:
			return fmt.Sprint(v)
		}
	}
	if val := c.Param("name"); val != "" {
		return val
	}
	return ""
}

func extractTraceOrderID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if orderID := c.Query("order_id"); orderID != "" {
		return orderID
	}
	if orderID := c.Param("order_id"); orderID != "" {
		return orderID
	}
	if orderID := c.Query("orderId"); orderID != "" {
		return orderID
	}
	return ""
}

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
