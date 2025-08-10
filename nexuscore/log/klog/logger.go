// package klog
// logger.go
// 提供klog与zap日志库的集成功能，实现将klog的日志输出重定向到zap日志实例。
// 核心功能包括：
// 1. 通过InitLogger函数初始化klog，将不同级别（INFO、WARNING、FATAL、ERROR）的日志输出绑定到zap对应的日志方法；
// 2. 自定义日志写入器（infoLogger、warnLogger等），实现klog日志到zap的转发；
// 3. 配置klog参数（如跳过日志头、关闭标准错误输出），确保与zap日志格式兼容。
// 适用于需要在使用k8s.io/klog的项目中统一日志输出到zap的场景，简化日志管理并保持日志格式一致性。
package klog

import (
	"flag"

	"go.uber.org/zap"
	"k8s.io/klog"
)

// InitLogger init klog by zap logger.
func InitLogger(zapLogger *zap.Logger) {
	fs := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(fs)
	defer klog.Flush()
	klog.SetOutputBySeverity("INFO", &infoLogger{logger: zapLogger})
	klog.SetOutputBySeverity("WARNING", &warnLogger{logger: zapLogger})
	klog.SetOutputBySeverity("FATAL", &fatalLogger{logger: zapLogger})
	klog.SetOutputBySeverity("ERROR", &errorLogger{logger: zapLogger})
	_ = fs.Set("skip_headers", "true")
	_ = fs.Set("logtostderr", "false")
}

type infoLogger struct {
	logger *zap.Logger
}

func (l *infoLogger) Write(p []byte) (n int, err error) {
	l.logger.Info(string(p[:len(p)-1]))
	return len(p), nil
}

type warnLogger struct {
	logger *zap.Logger
}

func (l *warnLogger) Write(p []byte) (n int, err error) {
	l.logger.Warn(string(p[:len(p)-1]))
	return len(p), nil
}

type fatalLogger struct {
	logger *zap.Logger
}

func (l *fatalLogger) Write(p []byte) (n int, err error) {
	l.logger.Fatal(string(p[:len(p)-1]))
	return len(p), nil
}

type errorLogger struct {
	logger *zap.Logger
}

func (l *errorLogger) Write(p []byte) (n int, err error) {
	l.logger.Error(string(p[:len(p)-1]))
	return len(p), nil
}
