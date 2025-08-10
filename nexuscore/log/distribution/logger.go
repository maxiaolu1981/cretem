/*
log:distribution:logger.go
让依赖logrus日志接口的代码能够无缝兼容底层基于zap的日志实现，同时支持标准库log和prometheus等组件的日志需求。
核心定位：日志接口适配层
结构体Logger同时持有*zap.Logger（底层高性能日志实现）和*logrus.Logger（兼容层），通过实现logrus风格的全套日志方法（如Debug()/Infof()/WithError()等），让原本依赖logrus接口的代码可以直接使用该Logger，而底层实际通过zap输出日志（兼顾性能和兼容性）。
方法适配细节
实现了logrus的核心日志方法：从Trace到Panic的各级别日志（含ln/f变体），例如Debugf()会将格式化字符串转换后通过zap.Logger.Debug()输出。
实现WithError()方法，返回logrus.Entry类型，完全兼容logrus的 “带错误上下文” 日志风格。
同时兼容标准库log的Print系列方法（Print()/Println()/Printf()），将其映射为zap的Info级别日志。
构造逻辑
通过NewLogger(logger *zap.Logger)创建实例时，会基于传入的zap.Logger生成一个logrus.Logger兼容实例（logruslogger.NewLogger(logger)），实现两种日志库的桥接。
*/

package distribution

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"

	logruslogger "github.com/maxiaolu1981/cretem/nexuscore/log/logrus"
)

// Logger is a logger which compatible to logrus/std log/prometheus.
// it implements Print() Println() Printf() Dbug() Debugln() Debugf() Info() Infoln() Infof() Warn() Warnln() Warnf()
// Error() Errorln() Errorf() Fatal() Fataln() Fatalf() Panic() Panicln() Panicf() With() WithField() WithFields().
type Logger struct {
	logger       *zap.Logger
	logrusLogger *logrus.Logger
}

// NewLogger create the field logger object by giving zap logger.
func NewLogger(logger *zap.Logger) *Logger {
	return &Logger{
		logger:       logger,
		logrusLogger: logruslogger.NewLogger(logger),
	}
}

// Print logs a message at level Print on the compatibleLogger.
func (l *Logger) Print(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Println logs a message at level Print on the compatibleLogger.
func (l *Logger) Println(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Printf logs a message at level Print on the compatibleLogger.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Trace logs a message at level Trace on the compatibleLogger.
func (l *Logger) Trace(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Traceln logs a message at level Trace on the compatibleLogger.
func (l *Logger) Traceln(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Tracef logs a message at level Trace on the compatibleLogger.
func (l *Logger) Tracef(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

// Debug logs a message at level Debug on the compatibleLogger.
func (l *Logger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Debugln logs a message at level Debug on the compatibleLogger.
func (l *Logger) Debugln(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Debugf logs a message at level Debug on the compatibleLogger.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

// Info logs a message at level Info on the compatibleLogger.
func (l *Logger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Infoln logs a message at level Info on the compatibleLogger.
func (l *Logger) Infoln(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Infof logs a message at level Info on the compatibleLogger.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Warn logs a message at level Warn on the compatibleLogger.
func (l *Logger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warnln logs a message at level Warn on the compatibleLogger.
func (l *Logger) Warnln(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warnf logs a message at level Warn on the compatibleLogger.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Warning logs a message at level Warn on the compatibleLogger.
func (l *Logger) Warning(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warningln logs a message at level Warning on the compatibleLogger.
func (l *Logger) Warningln(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Warningf logs a message at level Warning on the compatibleLogger.
func (l *Logger) Warningf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Error logs a message at level Error on the compatibleLogger.
func (l *Logger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Errorln logs a message at level Error on the compatibleLogger.
func (l *Logger) Errorln(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Errorf logs a message at level Error on the compatibleLogger.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// Fatal logs a message at level Fatal on the compatibleLogger.
func (l *Logger) Fatal(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}

// Fatalln logs a message at level Fatal on the compatibleLogger.
func (l *Logger) Fatalln(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}

// Fatalf logs a message at level Fatal on the compatibleLogger.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatal(fmt.Sprintf(format, args...))
}

// Panic logs a message at level Painc on the compatibleLogger.
func (l *Logger) Panic(args ...interface{}) {
	l.logger.Panic(fmt.Sprint(args...))
}

// Panicln logs a message at level Painc on the compatibleLogger.
func (l *Logger) Panicln(args ...interface{}) {
	l.logger.Panic(fmt.Sprint(args...))
}

// Panicf logs a message at level Painc on the compatibleLogger.
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.logger.Panic(fmt.Sprintf(format, args...))
}

// WithError return a logger with an error field.
func (l *Logger) WithError(err error) *logrus.Entry {
	return logrus.NewEntry(l.logrusLogger).WithError(err)
}
