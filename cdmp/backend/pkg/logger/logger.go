/*
logger 包是 GORM ORM 框架的定制日志适配器，实现了 gormlogger.Interface 接口，用于处理 GORM 操作（如 SQL 执行、错误、警告等）的日志输出。核心功能包括：
支持日志级别控制（Silent/Error/Warn/Info），按级别过滤日志；
定制日志格式（含颜色输出、执行时间、影响行数、SQL 语句等）；
跟踪慢查询（通过 SlowThreshold 配置阈值）；
自动记录日志调用的文件路径和行号，便于问题定位。
核心流程
该包围绕 GORM 日志的生成与输出 设计，关键流程如下：
1. 基础定义（常量与接口）
颜色常量：定义终端日志的颜色编码（如 Red 表示错误、Green 表示信息），用于彩色输出；
日志级别：定义 Silent（静默）、Error（错误）、Warn（警告）、Info（信息）四级，控制日志输出粒度；
Writer 接口：抽象日志输出器（Printf 方法），实际使用项目内部的 log.StdInfoLogger() 作为输出载体。
2. 配置与初始化（Config 与 New）
Config 结构体：存储日志配置，包括慢查询阈值（SlowThreshold）、是否彩色输出（Colorful）、日志级别（LogLevel）；
New 函数：根据输入的日志级别（level）初始化 logger 实例，预定义不同级别日志的格式字符串（含颜色模板），返回实现 gormlogger.Interface 的日志器。
3. 日志输出方法（Info/Warn/Error/Trace）
分级输出：Info/Warn/Error 分别处理对应级别的日志，根据当前日志级别判断是否输出（如 Warn 级日志仅在 LogLevel >= Warn 时输出）；
SQL 跟踪（Trace）：核心方法，记录 SQL 执行详情：
计算 SQL 执行耗时（elapsed）；
若执行出错且级别允许，输出错误日志（含错误信息、耗时、SQL）；
若耗时超过 SlowThreshold 且级别允许，输出慢查询警告；
正常执行时，输出常规 SQL 日志（含耗时、影响行数、SQL）。
4. 辅助功能（fileWithLineNum）
通过 runtime.Caller 获取日志调用的文件路径和行号，格式化后嵌入日志（如 logger/logger.go:123），便于定位日志产生的代码位置。
*/
package logger

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	gormlogger "gorm.io/gorm/logger" // GORM 日志接口定义

	"github.com/maxiaolu1981/cretem/nexuscore/log" // 项目内部日志输出器
)

// 定义日志颜色编码（终端输出用）
const (
	Reset       = "\033[0m"    // 重置颜色
	Red         = "\033[31m"   // 红色（错误）
	Green       = "\033[32m"   // 绿色（信息）
	Yellow      = "\033[33m"   // 黄色（警告/耗时）
	Blue        = "\033[34m"   // 蓝色
	Magenta     = "\033[35m"   // 品红
	Cyan        = "\033[36m"   // 青色
	White       = "\033[37m"   // 白色
	BlueBold    = "\033[34;1m" // 蓝色加粗（行数）
	MagentaBold = "\033[35;1m" // 品红加粗
	RedBold     = "\033[31;1m" // 红色加粗（慢查询）
	YellowBold  = "\033[33;1m" // 黄色加粗
)

// 定义 GORM 日志级别（从低到高）
const (
	Silent gormlogger.LogLevel = iota + 1 // 静默（不输出任何日志）
	Error                                 // 仅输出错误日志
	Warn                                  // 输出警告和错误日志
	Info                                  // 输出信息、警告和错误日志（最详细）
)

// Writer 日志输出器接口，定义日志打印方法
type Writer interface {
	Printf(string, ...interface{}) // 按格式输出日志
}

// Config 定义 GORM 日志器的配置项
type Config struct {
	SlowThreshold time.Duration       // 慢查询阈值（超过此时间的 SQL 会被标记为慢查询）
	Colorful      bool                // 是否启用彩色输出
	LogLevel      gormlogger.LogLevel // 日志级别（控制输出粒度）
}

// New 创建一个 GORM 日志器实例
// 参数 level：日志级别（对应 Silent/Error/Warn/Info）
// 返回：实现 gormlogger.Interface 的日志器
func New(level int) gormlogger.Interface {
	var (
		infoStr      = "%s[info] "                  // 信息日志格式（无颜色）
		warnStr      = "%s[warn] "                  // 警告日志格式（无颜色）
		errStr       = "%s[error] "                 // 错误日志格式（无颜色）
		traceStr     = "[%s][%.3fms] [rows:%v] %s"  // 常规 SQL 跟踪格式（无颜色）
		traceWarnStr = "%s %s[%.3fms] [rows:%v] %s" // 慢查询警告格式（无颜色）
		traceErrStr  = "%s %s[%.3fms] [rows:%v] %s" // 错误 SQL 跟踪格式（无颜色）
	)

	// 初始化配置（默认慢查询阈值 200ms，禁用彩色输出，日志级别由输入参数决定）
	config := Config{
		SlowThreshold: 200 * time.Millisecond,
		Colorful:      false,
		LogLevel:      gormlogger.LogLevel(level),
	}

	// 若启用彩色输出，更新格式字符串为带颜色的版本
	if config.Colorful {
		infoStr = Green + "%s " + Reset + Green + "[info] " + Reset
		warnStr = BlueBold + "%s " + Reset + Magenta + "[warn] " + Reset
		errStr = Magenta + "%s " + Reset + Red + "[error] " + Reset
		traceStr = Green + "%s " + Reset + Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
		traceWarnStr = Green + "%s " + Yellow + "%s " + Reset + RedBold + "[%.3fms] " + Yellow + "[rows:%v]" + Magenta + " %s" + Reset
		traceErrStr = RedBold + "%s " + MagentaBold + "%s " + Reset + Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
	}

	// 返回日志器实例，绑定输出器、配置和格式字符串
	return &logger{
		Writer:       log.StdInfoLogger(), // 使用项目默认的标准输出日志器
		Config:       config,
		infoStr:      infoStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceStr:     traceStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

// logger 实现 gormlogger.Interface 接口，是 GORM 日志器的具体实现
type logger struct {
	Writer                                     // 日志输出器（嵌入接口，继承 Printf 方法）
	Config                                     // 日志配置（嵌入结构体，直接使用其字段）
	infoStr, warnStr, errStr            string // 信息/警告/错误日志格式
	traceStr, traceErrStr, traceWarnStr string // SQL 跟踪相关格式
}

// LogMode 设置日志级别，返回新的日志器实例（不修改原实例）
func (l *logger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newlogger := *l            // 复制当前日志器
	newlogger.LogLevel = level // 更新日志级别
	return &newlogger
}

// Info 输出信息级别日志
// ctx：上下文（用于传递超时、追踪信息等）
// msg：日志消息
// data：格式化参数
func (l logger) Info(ctx context.Context, msg string, data ...interface{}) {
	// 仅当当前日志级别 >= Info 时输出
	if l.LogLevel >= Info {
		// 拼接日志格式：[文件:行号] + 信息前缀 + 消息，附加参数
		l.Printf(l.infoStr+msg, append([]interface{}{fileWithLineNum()}, data...)...)
	}
}

// Warn 输出警告级别日志
// 参数含义同 Info
func (l logger) Warn(ctx context.Context, msg string, data ...interface{}) {
	// 仅当当前日志级别 >= Warn 时输出
	if l.LogLevel >= Warn {
		l.Printf(l.warnStr+msg, append([]interface{}{fileWithLineNum()}, data...)...)
	}
}

// Error 输出错误级别日志
// 参数含义同 Info
func (l logger) Error(ctx context.Context, msg string, data ...interface{}) {
	// 仅当当前日志级别 >= Error 时输出
	if l.LogLevel >= Error {
		l.Printf(l.errStr+msg, append([]interface{}{fileWithLineNum()}, data...)...)
	}
}

// Trace 输出 SQL 执行跟踪日志，记录执行时间、影响行数、SQL 语句等
// ctx：上下文
// begin：SQL 执行开始时间
// fc：函数，返回 SQL 语句和影响行数
// err：执行过程中发生的错误
func (l logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// 若日志级别为 Silent（静默），直接返回
	if l.LogLevel <= 0 {
		return
	}

	elapsed := time.Since(begin) // 计算 SQL 执行耗时（毫秒）
	switch {
	// 情况 1：执行出错，且日志级别允许输出错误
	case err != nil && l.LogLevel >= Error:
		sql, rows := fc() // 获取 SQL 语句和影响行数
		if rows == -1 {
			// 未获取到行数（如查询失败），输出错误日志
			l.Printf(l.traceErrStr, fileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			// 输出错误日志（含影响行数）
			l.Printf(l.traceErrStr, fileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	// 情况 2：执行耗时超过慢查询阈值，且日志级别允许输出警告
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold) // 慢查询提示
		if rows == -1 {
			// 未获取到行数，输出慢查询警告
			l.Printf(l.traceWarnStr, fileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			// 输出慢查询警告（含影响行数）
			l.Printf(l.traceWarnStr, fileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	// 情况 3：正常执行，且日志级别允许输出信息
	case l.LogLevel >= Info:
		sql, rows := fc()
		if rows == -1 {
			// 未获取到行数，输出常规 SQL 日志
			l.Printf(l.traceStr, fileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			// 输出常规 SQL 日志（含影响行数）
			l.Printf(l.traceStr, fileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}

// fileWithLineNum 获取日志调用的文件路径（简化为包名+文件名）和行号
// 返回格式：package/file.go:line
func fileWithLineNum() string {
	// 遍历调用栈（从第 4 层开始，跳过 GORM 内部调用）
	for i := 4; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i) // 获取调用栈信息

		// 过滤测试文件（_test.go），仅保留业务代码
		if ok && !strings.HasSuffix(file, "_test.go") {
			dir, f := filepath.Split(file)                                              // 分离目录和文件名
			baseDir := filepath.Base(dir)                                               // 获取目录的基础名称（包名）
			return filepath.Join(baseDir, f) + ":" + strconv.FormatInt(int64(line), 10) // 格式化为 "包名/文件名:行号"
		}
	}

	return "" // 未找到有效调用位置时返回空
}
