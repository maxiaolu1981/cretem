/*
robfig/cron（Go 语言常用的定时任务库）的日志适配器，其核心作用是将基础库中封装的zap日志能力，适配到cron库的日志接口，确保定时任务的运行日志能通过统一的日志系统输出。
*/
package cronlog

import (
	"fmt"

	"go.uber.org/zap"
)

type logger struct {
	zapLogger *zap.SugaredLogger
}

// NewLogger create a logger which implement `github.com/robfig/cron.Logger`.
func NewLogger(zapLogger *zap.SugaredLogger) logger {
	return logger{zapLogger: zapLogger}
}

func (l logger) Info(msg string, args ...interface{}) {
	l.zapLogger.Infow(msg, args...)
}

func (l logger) Error(err error, msg string, args ...interface{}) {
	l.zapLogger.Errorw(fmt.Sprintf(msg, args...), "error", err.Error())
}

func (l logger) Flush() {
	_ = l.zapLogger.Sync()
}
