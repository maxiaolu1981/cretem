// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package log

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// 场景1：服务初始化与基础日志输出（带实际验证）
func TestServiceInitializationWithValidation(t *testing.T) {
	// 创建观察者核心用于捕获日志
	core, recordedLogs := observer.New(zapcore.InfoLevel)
	observedLogger := zap.New(core)

	// 替换全局日志器为观察者日志器
	originalLogger := logger
	logger = NewLogger(observedLogger).(*zapLogger)
	defer func() { logger = originalLogger }() // 测试后恢复

	// 执行日志操作
	Info("service_start", zap.String("status", "initializing"))
	Infof("config_loaded: port=%d, timeout=%ds", 8080, 30)
	Warnw("config_warning", "key", "max_conns", "value", 1000, "warning", "接近上限")

	// 验证日志数量
	if recordedLogs.Len() != 3 {
		t.Fatalf("预期3条日志，实际记录了%d条", recordedLogs.Len())
	}

	// 验证第一条日志
	firstLog := recordedLogs.All()[0]
	if firstLog.Message != "service_start" {
		t.Errorf("第一条日志消息错误，预期'service_start'，实际'%s'", firstLog.Message)
	}
	if firstLog.Level != zapcore.InfoLevel {
		t.Errorf("第一条日志级别错误，预期'info'，实际'%s'", firstLog.Level.String())
	}
	if firstLog.ContextMap()["status"] != "initializing" {
		t.Errorf("第一条日志缺少status字段，实际字段: %v", firstLog.ContextMap())
	}

	// 验证第三条警告日志
	warnLog := recordedLogs.All()[2]
	if warnLog.Level != zapcore.WarnLevel {
		t.Errorf("警告日志级别错误，预期'warn'，实际'%s'", warnLog.Level.String())
	}
	if warnLog.ContextMap()["key"] != "max_conns" {
		t.Errorf("警告日志缺少key字段，实际字段: %v", warnLog.ContextMap())
	}

	t.Log("服务初始化日志验证完成")
}

// 场景2：HTTP请求全链路日志传递（带实际验证）
func TestHTTPRequestTraceWithValidation(t *testing.T) {
	// 创建观察者
	core, recordedLogs := observer.New(zapcore.DebugLevel)
	observedLogger := zap.New(core)

	// 替换全局日志器
	originalLogger := logger
	logger = NewLogger(observedLogger).(*zapLogger)
	defer func() { logger = originalLogger }()

	// 执行HTTP请求链路日志操作
	reqID := "req-20240807-123"
	reqLogger := WithValues("request_id", reqID, "method", "POST", "path", "/api/v1/user")
	reqLogger.Info("request_received")

	userLogger := reqLogger.WithName("user_service").WithValues("user_id", "u789")
	userLogger.Info("user_query_start")

	dbLogger := userLogger.WithName("db")
	dbLogger.V(1).Info("sql_executed", "sql", "SELECT * FROM users")

	// 验证日志数量
	if recordedLogs.Len() != 3 {
		t.Fatalf("预期3条日志，实际记录了%d条", recordedLogs.Len())
	}

	// 验证所有日志都包含request_id
	for i, log := range recordedLogs.All() {
		if log.ContextMap()["request_id"] != reqID {
			t.Errorf("第%d条日志缺少request_id，实际字段: %v", i+1, log.ContextMap())
		}
	}

	// 验证数据库日志的模块名和级别
	dbLog := recordedLogs.All()[2]
	if dbLog.LoggerName != "user_service.db" {
		t.Errorf("数据库日志模块名错误，预期'user_service.db'，实际'%s'", dbLog.LoggerName)
	}
	if dbLog.Level != zapcore.DebugLevel {
		t.Errorf("数据库日志级别错误，预期'debug'，实际'%s'", dbLog.Level.String())
	}

	t.Log("HTTP请求链路日志验证完成")
}

// 场景3：错误处理与日志级别控制（带实际验证）
func TestErrorHandlingWithValidation(t *testing.T) {
	// 创建观察者（支持所有级别）
	core, recordedLogs := observer.New(zapcore.DebugLevel)
	observedLogger := zap.New(core)

	// 替换全局日志器
	originalLogger := logger
	logger = NewLogger(observedLogger).(*zapLogger)
	defer func() { logger = originalLogger }()

	orderLogger := WithName("order").WithValues("order_id", "ord-456")

	// 输出不同级别日志
	Warn("inventory_warn", zap.String("error", "insufficient_stock"))
	orderLogger.Error(errors.New("payment_timeout"), "payment_failed")
	orderLogger.V(1).Info("debug_info") // 调试级别

	// 验证日志数量和级别
	if recordedLogs.Len() != 3 {
		t.Fatalf("预期3条日志，实际记录了%d条", recordedLogs.Len())
	}

	logLevels := []zapcore.Level{zapcore.WarnLevel, zapcore.ErrorLevel, zapcore.DebugLevel}
	for i, log := range recordedLogs.All() {
		if log.Level != logLevels[i] {
			t.Errorf("第%d条日志级别错误，预期'%s'，实际'%s'",
				i+1, logLevels[i].String(), log.Level.String())
		}
	}

	// 验证错误日志包含错误信息
	errorLog := recordedLogs.All()[1]
	if errorLog.ContextMap()["error"] != "payment_timeout" {
		t.Errorf("错误日志缺少错误信息，实际字段: %v", errorLog.ContextMap())
	}

	t.Log("错误处理日志验证完成")
}

// 场景4：批量任务处理日志（带实际验证）
func TestBatchTaskLoggingWithValidation(t *testing.T) {
	// 创建观察者
	core, recordedLogs := observer.New(zapcore.InfoLevel)
	observedLogger := zap.New(core)

	// 替换全局日志器
	originalLogger := logger
	logger = NewLogger(observedLogger).(*zapLogger)
	defer func() { logger = originalLogger }()

	batchID := "batch-999"
	batchLogger := WithValues("batch_id", batchID)
	batchLogger.Info("batch_start")

	// 处理3个任务（使用int64类型的task_id，与Zap的默认处理一致）
	for taskIdx := int64(1); taskIdx <= 3; taskIdx++ {
		taskLogger := batchLogger.WithValues("task_id", taskIdx)
		taskLogger.Info("task_completed")
	}

	// 验证日志数量
	if recordedLogs.Len() != 4 {
		t.Fatalf("预期4条日志，实际记录了%d条", recordedLogs.Len())
	}

	// 验证所有日志都包含batch_id
	for i, log := range recordedLogs.All() {
		val, ok := log.ContextMap()["batch_id"]
		if !ok || val != batchID {
			t.Errorf("第%d条日志缺少或错误的batch_id，实际值: %v", i+1, val)
		}
	}

	// 验证任务日志包含正确的task_id（适配int64类型）
	for i := int64(1); i <= 3; i++ {
		taskLog := recordedLogs.All()[int(i)]
		// 从上下文获取task_id并进行类型转换
		val, ok := taskLog.ContextMap()["task_id"]
		if !ok {
			t.Errorf("第%d个任务日志缺少task_id", i)
			continue
		}
		// 将interface{}转换为int64类型（关键修复，匹配Zap的默认处理）
		taskID, ok := val.(int64)
		if !ok {
			t.Errorf("第%d个任务日志的task_id类型错误，实际类型: %T", i, val)
			continue
		}
		if taskID != i {
			t.Errorf("第%d个任务日志的task_id值错误，预期%d，实际%d", i, i, taskID)
		}
	}

	t.Log("批量任务日志验证完成")
}
