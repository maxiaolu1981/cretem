/*
// Copyright 2025 马晓璐 <15940995655@13..com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
log:log.go
基于 Uber 的高性能日志库 zap 封装实现，提供了一套功能完整、接口统一的结构化日志解决方案。
该包的核心设计目标是：
标准化日志接口：通过 Logger 和 InfoLogger 接口定义日志操作规范，隔离业务代码与底层日志库（zap）的强依赖，便于未来扩展或替换日志实现。
支持分级日志：通过 V(level int) 方法实现日志详细程度的控制（级别 0 为核心日志，级别 1 为调试日志），适配开发 / 生产环境的不同需求。
结构化日志输出：支持键值对、格式化字符串、zap.Field 等多种输入形式，输出 JSON 或控制台格式的结构化日志，便于日志分析工具（如 ELK）解析。
上下文扩展能力：通过 WithValues（附加键值对）和 WithName（附加模块名称）创建子日志器，实现日志上下文的复用与传递，适合分布式追踪、多模块日志隔离等场景。
兼容性集成：支持与 Kubernetes 生态的 klog 日志库、标准库 log 集成，确保不同组件的日志输出统一。
*/
package log

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/maxiaolu1981/cretem/nexuscore/log/klog"
)

/*
非错误日志包括
状态记录：程序启动、初始化完成、模块加载成功等正常状态。
例如："服务已启动，监听端口 8080"
操作跟踪：用户登录、数据查询、任务开始 / 结束等业务行为。
例如："用户 admin 登录成功，IP: 192.168.1.1"
性能指标：耗时统计、资源占用等监控信息。
例如："查询用户列表完成，耗时 200ms"
调试信息：开发或测试阶段输出的变量值、流程分支等细节（Debug 级别）。
*/
// InfoLogger 定义非错误日志的接口，支持指定详细程度输出日志
type InfoLogger interface {
	//msg 应为固定描述（如 “用户登录”“数据同步完成”），用于标识日志的核心事件。
	//键值对（key/value pairs）用于携带动态变量（如 user_id: 123、cost: 200ms），提供事件的具体上下文。
	Info(msg string, keysAndValues ...interface{}) // 输出非错误日志（带键值对）
	Infof(format string, args ...interface{})      // 格式化输出非错误日志

	//Enabled 方法用于检查当前 InfoLogger 是否启用。例如，
	// 可以通过命令行标志来设置日志的详细程度，从而禁用某些信息日志。
	//举例来说：如果通过命令行参数 --log.level=warn 将日志级别设为 warn，那么 Info 级别的日志会被禁用，此时 Enabled() 会返回 false。
	Enabled() bool
}

// Logger 定义完整日志接口，包含错误日志和分级日志功能
type Logger interface {
	// 所有 Logger 都实现了 InfoLogger 接口。直接在 Logger 实例上调用 InfoLogger 的方法，
	// 等效于在 V (0) 级别的 InfoLogger 上调用这些方法。例如，logger.Info() 的效果
	// 与 logger.V (0).Info () 完全相同。
	//V 是 Logger 接口中的一个方法，定义为 V(level int) InfoLogger，调用后会返回一个对应级别的 InfoLogger 实例。
	//整数 level 表示 “详细程度等级”：
	//级别越低（如 0），日志越基础、越重要，默认情况下通常会输出。
	//级别越高（如 1、2），日志越详细、越冗余（可能包含调试信息），通常在开发或调试时才启用
	InfoLogger
	// Error 方法用于记录错误信息，同时附带给定的消息和键值对作为上下文。
	// 它的功能类似于调用 Info 方法并传入名为 "error" 的值，但可能具有独特的行为，因此记录错误时应优先使用此方法（更多信息请参见包文档）
	//msg 字段应用于为底层错误添加上下文描述，
	// 而 err 字段则应用于附加触发这条日志的实际错误对象（如果存在）。
	//使用例子
	//	err := db.Query(...)
	//if err != nil {
	//    logger.Error(err, "查询用户数据失败", "user_id", 123)
	//}
	Error(err error, msg string, keysAndValues ...interface{})
	Errorf(format string, args ...interface{})

	/*V 返回指定详细程度的 InfoLogger（级别越高，日志越不重要）
	关键说明：
	级别与重要性的关系：
	这里明确了 “详细级别（verbosity level）” 与日志重要性的反比关系：
	低级别（如 V(0)）：日志更重要（如核心业务流程、关键状态），默认会输出。
	高级别（如 V(2)）：日志更次要（如调试细节、非关键中间状态），通常仅在需要详细排查时启用。
	参数合法性：
	禁止传入负数级别（如 V(-1)），这是不被允许的，可能会导致 panic 或日志行为异常（代码中通常会有校验）。
	例如，V(0) 用于核心信息，V(1) 用于次要信息，V(2) 用于调试细节，级别越高，日志越琐碎，重要性越低。
	*/
	V(level int) InfoLogger
	// Write 实现 io.Writer 接口，支持将字节流作为日志输出
	Write(p []byte) (n int, err error)

	// WithValues 为日志器添加键值对形式的上下文信息
	WithValues(keysAndValues ...interface{}) Logger

	// WithName 方法为日志器的名称添加一个新的元素。
	// 多次调用 WithName 会持续向日志器的名称追加后缀。
	// 强烈建议名称片段仅包含字母、数字和连字符
	//功能作用：WithName 用于给日志器添加 “命名前缀”，便于区分不同模块或功能的日志。例如，在用户模块中使用 logger.WithName("user")，生成的日志会带上 user 标识。
	//多次调用会累积名称，通常用点分隔。例如：
	//logger := log.NewLogger(...)
	//logger.WithName("user").WithName("login") // 最终名称可能为 "user.login"
	WithName(name string) Logger

	// WithContext 将日志器关联到上下文，便于在上下文传递中获取日志器
	/*
				在 Go 语言中，“上下文（context）” 指的是 context.Context 类型的对象，它主要用于在函数调用链、goroutine 之间传递截止时间、取消信号和请求范围的元数据。简单来说，上下文就像一个 “信息载体”，随着程序的执行流程在不同函数或协程间传递，携带一些全局或请求相关的信息。
				当我们调用 logger.WithContext(ctx) 时，实际上是将当前日志器（logger）与一个上下文对象（ctx）关联起来，并返回一个新的上下文。这样做的目的是：
			让日志器能够随着上下文在函数调用链中传递，避免手动在每个函数参数中显式传递日志器。
		例如，在处理一个 HTTP 请求时：
		收到请求后，创建一个初始上下文 ctx 和一个带请求 ID 的日志器 logger := baseLogger.WithValues("request_id", "123")。
		通过 ctx = logger.WithContext(ctx) 将日志器关联到上下文。
		在后续的函数调用（如 handleRequest(ctx)、queryDB(ctx) 等）中，只需传递 ctx，就能通过 ctx 取出关联的日志器，继续添加上下文信息或输出日志。

	*/
	WithContext(ctx context.Context) context.Context

	/*
			// Flush 方法会调用底层 Core 的 Sync 方法，将所有缓冲的日志条目刷新到输出目标。
			// 应用程序在退出前应当注意调用 Sync 方法（确保日志完整输出）。
			Flush()
			关键说明：
		日志库通常会对日志进行缓冲（减少频繁 IO 操作，提升性能），即日志不是立即写入文件 / 控制台，而是先暂存在内存中，批量写入。
		Flush 方法的作用就是强制将内存中缓冲的日志立即写入输出目标（如文件、stdout 等），避免因程序意外退出导致日志丢失。
		特别强调 “程序退出前需调用”：例如在服务关闭、进程终止时，必须调用 Flush（或通过 logger.Flush()），否则可能丢失最后一批未写入的日志。
		举例：在 HTTP 服务关闭时，通常会在 Shutdown 钩子中调用 log.Flush()，确保服务终止前的所有日志都被正确记录。
	*/
}

// 确保 zapLogger 实现 Logger 接口（编译期检查
var _ Logger = &zapLogger{}
var disabledInfoLogger = &noopInfoLogger{}

// noopInfoLogger 是一个禁用的 InfoLogger，所有方法均为空实现
type noopInfoLogger struct{}

func (l *noopInfoLogger) Enabled() bool                    { return false }
func (l *noopInfoLogger) Info(_ string, _ ...interface{})  {}
func (l *noopInfoLogger) Infof(_ string, _ ...interface{}) {}

/*
// 注意：目前，我们始终使用等效于 “sugared logging”（简化日志）的方式。
// 这是必要的，因为 logr 没有定义非简化的日志类型，
// 而使用 Zap 特有的非简化类型会导致代码直接与 Zap 绑定（不利于替换日志库）。
//infoLogger 是 logr.InfoLogger 接口的实现，它使用 Zap 按特定级别输出日志。
// 其中的级别已经转换为 Zap 的级别，也就是说：logr 级别 = -1 × Zap 级别。
logr 的级别定义与 Zap 相反：logr 级别越高（如 V(2)），日志越详细、重要性越低；
*/
type infoLogger struct {
	level zapcore.Level // 日志级别（Zap 级别，与 logr 级别相反：logrLevel = -1*zapLevel）
	log   *zap.Logger   // 底层 Zap 日志器
}

// Enabled 始终返回 true（当前级别已通过检查）
func (l *infoLogger) Enabled() bool { return true }

/*
// 1. 检查当前级别 级别是否是否允许输出该日志，并创建一个日志条目
 2. 若允许输出，则处理键值对并写入日志
    当调用 logger.V(0).Info("用户登录", "user_id", 123, "ip", "192.168.1.1") 时：
    l.log.Check(l.level, "用户登录") 检查 V(0) 对应的级别（通常是 Info）是否启用，若启用则返回 checkedEntry。
    handleFields 将 ["user_id", 123, "ip", "192.168.1.1"] 转换为 Zap 字段（zap.Int("user_id", 123), zap.String("ip", "192.168.1.1")）。
    checkedEntry.Write(...) 输出完整日志，包含消息 “用户登录”、用户 ID、IP 等上下文信息。
    举例说明

假设全局配置的日志级别是 Info（这是很多系统的默认值）：
V(0) 对应的级别是 Info（根据代码中 logrLevel = -1*zapLevel 的转换规则），与全局阈值相等 → 启用，日志会输出。
V(1) 对应的级别是 Debug（比 Info 更低），低于全局阈值 → 未启用，日志会被过滤掉。
当调用 l.log.Check(l.level, "用户登录") 时：
如果 l.level（如 Info）≥ 全局配置级别（如 Info），Check 方法返回非 nil 的日志条目 → 日志会被输出（启用状态）。
如果 l.level（如 Debug）< 全局配置级别（如 Info），Check 方法返回 nil → 日志不会输出（未启用状态）。
核心目的
通过 “级别启用” 的判断，可以灵活控制日志的详细程度：
开发 / 调试时，可将全局级别设为 Debug（启用更低级别日志），输出更多细节。
生产环境中，可将全局级别设为 Info 或 Warn（禁用低级别日志），减少冗余输出，提升性能。
简单说，“级别启用” 就是日志系统的一道 “闸门”，只允许重要程度足够高的日志通过
*/
func (l *infoLogger) Info(msg string, keysAndValues ...interface{}) {

	if checkedEntry := l.log.Check(l.level, msg); checkedEntry != nil {
		checkedEntry.Write(handleFields(l.log, keysAndValues)...)
	}
}

// Infof 格式化输出非错误日志（使用 Zap 的 SugaredLogger）
/*
Info 与 Infof 的本质区别
Info 方法是手动调用 l.log.Check(l.level, msg) 显式检查级别，然后决定是否输出（这种方式更灵活，可在输出前做额外处理）。
Infof 方法使用了 Zap 的 SugaredLogger（简化日志接口），它的 Infof 方法内部会自动检查级别（逻辑与 Check 类似）：如果级别未启用，会直接忽略日志输出。
*/
func (l *infoLogger) Infof(format string, args ...interface{}) {
	l.log.Sugar().Infof(format, args...)
}

// zapLogger 是 Logger 接口的实现，封装 Zap 日志器核心功能
type zapLogger struct {
	// NB: this looks very similar to zap.SugaredLogger, but
	// deals with our desire to have multiple verbosity levels.
	zapLogger  *zap.Logger // 底层 Zap 日志器
	infoLogger             // 内嵌 infoLogger，实现 InfoLogger 接口
}

/*
作用：将用户传入的 “键值对列表”（如["user_id", 123]）转换为 Zap 识别的zap.Field类型，并进行合法性校验。
type Field struct {
    Key       string      // 字段的键（必须是字符串）
    Type      fieldType   // 值的类型（如字符串、整数、布尔等）
    Integer   int64       // 存储整数类型的值
    String    string      // 存储字符串类型的值
    Interface interface{} // 存储其他复杂类型的值
}
核心逻辑：
校验键值对合法性：禁止传入zap.Field类型（避免与 Zap 强绑定）、确保键为字符串、键值对数量成对。
转换键值对：通过zap.Any(keyStr, val)将键值对转换为zap.Field。
合并字段：将转换后的字段与预定义附加字段（如错误字段）合并返回。
举例：["user_id", 123] → []zap.Field{zap.Any("user_id", 123)}。
参数：
l *zap.Logger：zap 日志实例，用于在出现错误时记录 panic 级别的日志（DPanic）。
args []interface{}：输入的键值对参数（通常是日志的额外信息，如 key1, val1, key2, val2 形式）。
additional ...zap.Field：已有的结构化日志字段（需要与 args 转换后的字段合并）。
返回值：[]zap.Field，合并后的结构化日志字段，可直接用于 zap 日志输出。
*/

func handleFields(l *zap.Logger, args []interface{}, additional ...zap.Field) []zap.Field {
	//如果没有输入键值对,则返回additional
	if len(args) == 0 {
		return additional
	}

	//创建一个 zap.Field 类型的切片 fields，用于存储 args 转换后的字段。
	//初始容量设为 len(args)/2 + len(additional)：
	//len(args)/2 是因为 args 是键值对形式（每个键值对占 2 个元素），预估最大字段数。
	//加上 len(additional) 是为了预留合并已有字段的空间，减少动态扩容的内存开销。
	fields := make([]zap.Field, 0, len(args)/2+len(additional))
	for i := 0; i < len(args); {

		//logr 是一个抽象的日志接口，不依赖具体实现（如 zap），因此不允许直接传入 zap.Field 这种强类型字段。
		//如果检测到 args[i] 是 zap.Field 类型，通过 l.DPanic 记录错误（开发环境会 panic，生产环境仅记录日志），并终止循环（不再处理后续参数）。
		if _, ok := args[i].(zap.Field); ok {
			l.DPanic("strongly-typed Zap Field passed to logr", zap.Any("zap field", args[i]))
			break
		}

		// 检查键值对数量是否匹配（奇数个参数）
		//键值对应成对出现（键 + 值），如果 i 是最后一个索引（即 args 长度为奇数），说明存在孤立的键（没有对应的值）。
		//此时记录错误，并终止循环（忽略该孤立键和后续参数）。
		/*
			len(args) = 5，因此len(args)-1 = 4（最后一个元素的索引是 4）。
			循环过程：
			i=0：处理args[0]（"name"）和args[1]（"Alice"），正常。
			i=2：处理args[2]（"age"）和args[3]（30），正常。
			i=4：此时i == len(args)-1（4 == 4），说明args[4]（"gender"）是最后一个元素，没有对应的 “值”，触发DPanic。
		*/
		if i == len(args)-1 {
			l.DPanic("odd number of arguments passed as key-value pairs for logging", zap.Any("ignored key", args[i]))
			break
		}

		//  处理合法的键值对
		// ensuring that the key is a string
		key, val := args[i], args[i+1]
		//提取键（key）和值（val），并检查键是否为字符串类型（日志的键通常必须是字符串）。
		keyStr, isString := key.(string)
		if !isString {
			// if the key isn't a string, DPanic and stop logging
			l.DPanic("non-string key argument passed to logging, ignoring all later arguments", zap.Any("invalid key", key))
			break
		}
		//如果合法，通过 zap.Any(keyStr, val) 创建一个 zap.Field（zap.Any 支持任意类型的值），并添加到 fields 中。
		fields = append(fields, zap.Any(keyStr, val))
		i += 2
	}
	return append(fields, additional...)
}

var (
	logger  *zapLogger
	options *Options
)

// nolint: gochecknoinits // need to init a default logger
func init() {
	Init(NewOptions())
}

// 日志初始化（全局配置）
// 用于初始化全局日志配置，将用户传入的自定义参数（如日志级别、输出格式、是否启用颜色等）应用到 zap 日志库，并创建全局唯一的日志器实例（logger），供后续所有日志操作使用。
func Init(opts *Options) {
	options = opts

	// 1. 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",                    // 日志中时间字段的键名
		LevelKey:      "level",                   // 日志中级别字段的键名
		NameKey:       "logger",                  // 日志中记录器名称字段的键名
		CallerKey:     "caller",                  // 日志中调用位置（文件:行号）字段的键名
		MessageKey:    "msg",                     // 日志中消息内容字段的键名
		StacktraceKey: "stack",                   // 日志中堆栈跟踪字段的键名
		LineEnding:    zapcore.DefaultLineEnding, // 日志行的结束符（默认是换行符）

		EncodeLevel:    zapcore.LowercaseLevelEncoder, // 日志级别编码方式（这里使用小写字母，如debug/info/error）
		EncodeTime:     timeEncoder,                   // 时间字段的编码函数（自定义函数，通常格式化时间为可读格式）
		EncodeDuration: milliSecondsDurationEncoder,   //  duration字段的编码函数（这里转为毫秒）
		EncodeCaller:   zapcore.ShortCallerEncoder,    // 调用位置的编码方式（这里使用短路径，如log/init.go:23）
	}

	// 2. 解析日志级别
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(opts.Level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// 3. 根据Format选择编码器
	var encoder zapcore.Encoder
	switch strings.ToLower(opts.Format) {
	case consoleFormat:
		encoder = zapcore.NewConsoleEncoder(encoderConfig) // 控制台格式
	case jsonFormat:
		encoder = zapcore.NewJSONEncoder(encoderConfig) // JSON格式
	default:
		panic(fmt.Sprintf("不支持的日志格式: %s（仅支持console/json）", opts.Format))
	}

	// 4. 初始化普通日志写入器（Info及以下级别）
	stdOutWriters := make([]zapcore.WriteSyncer, 0, len(opts.OutputPaths))
	for _, path := range opts.OutputPaths {
		switch path {
		case "stdout":
			stdOutWriters = append(stdOutWriters, zapcore.AddSync(os.Stdout))
		case "stderr":
			stdOutWriters = append(stdOutWriters, zapcore.AddSync(os.Stderr))
		default:
			// 确保目录存在
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				panic(fmt.Sprintf("创建普通日志目录失败: %v", err))
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				panic(fmt.Sprintf("打开普通日志文件失败: %v", err))
			}
			stdOutWriters = append(stdOutWriters, zapcore.AddSync(f))
		}
	}
	stdOutSyncer := zapcore.NewMultiWriteSyncer(stdOutWriters...)

	// 5. 初始化错误日志写入器（Error及以上级别）
	errWriters := make([]zapcore.WriteSyncer, 0, len(opts.ErrorOutputPaths))
	for _, path := range opts.ErrorOutputPaths {
		switch path {
		case "stdout":
			errWriters = append(errWriters, zapcore.AddSync(os.Stdout))
		case "stderr":
			errWriters = append(errWriters, zapcore.AddSync(os.Stderr))
		default:
			// 确保目录存在
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				panic(fmt.Sprintf("创建错误日志目录失败: %v", err))
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				panic(fmt.Sprintf("打开错误日志文件失败: %v", err))
			}
			errWriters = append(errWriters, zapcore.AddSync(f))
		}
	}
	errSyncer := zapcore.NewMultiWriteSyncer(errWriters...)

	// 6. 创建双核心：普通日志核心 + 错误日志核心
	stdCore := zapcore.NewCore(
		encoder,
		stdOutSyncer,
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl < zapcore.ErrorLevel && lvl >= zapLevel
		}),
	)

	errCore := zapcore.NewCore(
		encoder,
		errSyncer,
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel && lvl >= zapLevel
		}),
	)

	// 7. 合并核心并创建日志器
	core := zapcore.NewTee(stdCore, errCore)
	l := zap.New(core,
		zap.AddStacktrace(zapcore.PanicLevel),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	)

	// 8. 初始化全局日志器
	logger = &zapLogger{
		zapLogger: l,
		infoLogger: infoLogger{
			log:   l,
			level: zapcore.InfoLevel,
		},
	}

	// 9. 初始化其他集成
	klog.InitLogger(l)
	zap.RedirectStdLog(l)
}

// 返回标准库log.Logger实例，分别映射到 zap 的 Error 级别（用于兼容标准库日志）。
// 关键逻辑：
// 通过zap.NewStdLogAt将标准库日志的输出转换为 zap 的Error级别日志，使得使用标准库log包打印的日志（如log.Println）会被 zap 以错误级别记录，统一日志格式和输出目标。
func StdErrLogger() *log.Logger {
	if logger == nil {
		return nil
	}
	if l, err := zap.NewStdLogAt(logger.zapLogger, zapcore.ErrorLevel); err == nil {
		return l
	}
	return nil
}

// 返回标准库log.Logger实例，分别映射到 zap 的 info 级别（用于兼容标准库日志）。
func StdInfoLogger() *log.Logger {
	if logger == nil {
		return nil
	}
	if l, err := zap.NewStdLogAt(logger.zapLogger, zapcore.InfoLevel); err == nil {
		return l
	}
	return nil
}

// 输出错误级别日志，附加错误对象和上下文键值对。
func (l *zapLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	if checkedEntry := l.zapLogger.Check(zap.ErrorLevel, msg); checkedEntry != nil {
		checkedEntry.Write(handleFields(l.zapLogger, keysAndValues, zap.Error(err))...)
	}
}

// 直接调用底层zap.Logger的SugaredLogger（简化日志接口）的Errorf方法，格式化args并输出错误日志。
// （SugaredLogger自动处理级别检查和格式化，无需手动调用Check）。
func (l *zapLogger) Errorf(format string, args ...interface{}) {
	l.log.Sugar().Errorf(format, args...)
}

// 根据指定的 “详细级别”（level）返回对应的InfoLogger，实现日志的分级输出（级别越高，日志越详细，默认可能不输出）。
func V(level int) InfoLogger { return logger.V(level) }

// V(0) → 对应 zap 的 Info 级别（日常核心日志）。
// V(1) → 对应 zap 的 Debug 级别（详细调试日志）。
func (l *zapLogger) V(level int) InfoLogger {
	if level < 0 || level > 1 {
		panic("valid log level is [0, 1]")
	}
	lvl := zapcore.Level(-1 * level)
	if l.zapLogger.Core().Enabled(lvl) {
		return &infoLogger{
			level: lvl,
			log:   l.zapLogger,
		}
	}
	return disabledInfoLogger
}

// 实现io.Writer接口，支持将字节流（如其他组件的输出）作为日志输出。
func (l *zapLogger) Write(p []byte) (n int, err error) {
	l.zapLogger.Info(string(p))
	return len(p), nil
}

// 基于全局日志器创建带上下文的子日志器，简化通用场景调用。
func WithValues(keysAndValues ...interface{}) Logger { return logger.WithValues(keysAndValues...) }

/*子日志器的意义
子日志器的核心意义
上下文复用与传递
子日志器可以携带固定的上下文信息（如请求 ID、用户 ID、模块名称等），避免在每次打印日志时重复添加这些字段。例如，在一个 HTTP 请求的处理链路中，创建一个携带request_id的子日志器，后续所有该请求相关的日志都会自动包含这个 ID，无需在每个日志语句中手动传入。
模块化日志隔离
不同模块（如数据库层、缓存层、业务逻辑层）可以创建各自的子日志器，并附加模块名称（如WithName("db")），便于在日志中快速定位来源，或针对特定模块单独调整日志级别（如果系统支持）。
保持配置一致性
子日志器继承父日志器的核心配置（如输出到文件还是控制台、日志格式是 JSON 还是文本），避免因重复配置导致的不一致问题，同时简化日志系统的维护成本。
应用场景举例
1. 分布式追踪与请求链路日志
在分布式系统中，一个用户请求可能经过多个服务或模块。通过子日志器传递全局唯一的trace_id，可以将整个链路的日志串联起来，快速定位问题。
go
// 父日志器
parentLogger := zap.NewProduction()

// 接收到请求时，创建携带trace_id的子日志器
traceID := "abc123"
reqLogger := parentLogger.With(zap.String("trace_id", traceID))

// 后续所有该请求的日志都会包含trace_id
reqLogger.Info("开始处理请求")
// 输出：{"level":"info","trace_id":"abc123","msg":"开始处理请求"}

// 调用下游模块时，传递子日志器
processData(reqLogger)
2. 模块专属日志
为不同代码模块创建带模块名称的子日志器，便于日志筛选和分析。
go
// 父日志器
rootLogger := zap.NewProduction()

// 数据库模块子日志器
dbLogger := rootLogger.Named("database")
dbLogger.Info("连接数据库成功")
// 输出：{"level":"info","logger":"database","msg":"连接数据库成功"}

// 缓存模块子日志器
cacheLogger := rootLogger.Named("cache")
cacheLogger.Warn("缓存命中率低")
// 输出：{"level":"warn","logger":"cache","msg":"缓存命中率低"}
3. 临时上下文附加
在处理特定业务逻辑时（如批量任务），为子日志器附加临时上下文（如任务 ID），便于追踪单个任务的执行过程。
go
// 父日志器
taskLogger := zap.NewProduction()

// 处理第100个任务时，创建带task_id的子日志器
taskID := 100
subLogger := taskLogger.With(zap.Int("task_id", taskID))

subLogger.Info("任务开始执行")
// 输出：{"level":"info","task_id":100,"msg":"任务开始执行"}

// 任务出错时，日志自动包含task_id
subLogger.Error("任务执行失败", zap.Error(err))
// 输出：{"level":"error","task_id":100,"error":"xxx","msg":"任务执行失败"}
4. 日志级别局部调整（部分框架支持）
部分日志框架中，子日志器可以在父日志器的基础上单独调整日志级别（如父日志器为info，子日志器可设为debug），用于临时调试特定模块，而不影响全局日志输出。
总结
子日志器的核心价值在于 **“上下文复用” 和 “模块化隔离”**，通过在父日志器基础上附加专属信息，既能保证日志配置的一致性，又能大幅提升日志的可读性和可追踪性，尤其在复杂系统（如分布式服务、多模块应用）中作用显著。

*/
//基于当前日志器实例（可能是全局日志器或子日志器）创建新的子日志器，支持链式扩展。
//步骤1：转换键值对为zap结构化字段
// 步骤2：基于当前日志器的底层zap.Logger创建子日志器
// 子日志器会继承父日志器的所有配置（级别、格式等），并附加新字段
func (l *zapLogger) WithValues(keysAndValues ...interface{}) Logger {
	newLogger := l.zapLogger.With(handleFields(l.zapLogger, keysAndValues)...)
	return NewLogger(newLogger)
}

/*
一、WithName 与 (l *zapLogger) WithName 的意义
WithName（全局函数）和 (l *zapLogger) WithName（结构体方法）的核心作用是为日志器添加 “命名前缀”，用于标识日志所属的模块、组件或功能，实现日志的模块化隔离。
本质：通过 WithName 创建的子日志器会继承父日志器的所有配置（级别、格式等），但会在日志中附加一个 logger 字段（对应 EncoderConfig.NameKey），其值为累积的命名前缀（多组名称以 . 分隔）。
意义：
快速定位日志来源：通过 logger 字段可直接区分日志来自哪个模块（如 user.login、order.pay）。
便于日志筛选：在日志分析工具（如 ELK）中，可通过 logger 字段筛选特定模块的日志。
package main

import (

	"errors"
	"your/log/package/path" // 替换为实际的日志包路径

)

	func main() {
		// 1. 初始化全局日志器（假设已通过 Init 配置）
		// log.Init(log.NewOptions())

		// 2. 创建用户模块的子日志器（命名前缀为 "user"）
		userLogger := log.WithName("user")
		// 模拟用户登录流程：附加用户 ID 上下文
		loginLogger := userLogger.WithValues("user_id", "u12345", "ip", "192.168.1.1")
		loginLogger.Info("用户登录成功")

		// 3. 创建订单模块的子日志器（命名前缀为 "order"）
		orderLogger := log.WithName("order")
		// 模拟订单支付流程：附加订单 ID 和用户 ID 上下文
		payLogger := orderLogger.WithValues("order_id", "o67890", "user_id", "u12345")
		payLogger.Info("订单支付开始")

		// 4. 模拟支付失败场景
		payErr := errors.New("余额不足")
		payLogger.Error(payErr, "订单支付失败")
	}
*/
func WithName(s string) Logger {
	return logger.WithName(s)
}

func (l *zapLogger) WithName(name string) Logger {
	newLogger := l.zapLogger.Named(name)
	return NewLogger(newLogger)
}

/*
日志刷新：Flush() 与 (l *zapLogger) Flush()
含义：强制将日志缓冲区中的数据写入输出目标（如文件、控制台），避免程序退出时日志丢失。
底层实现：调用 zap.Logger.Sync() 方法（zap 库的同步接口），确保缓冲日志落地。
关键特性：忽略同步错误（_ = l.zapLogger.Sync()），因刷新失败通常不影响程序核心逻辑，但需在程序退出前调用。
*/
func Flush() { logger.Flush() }
func (l *zapLogger) Flush() {
	_ = l.zapLogger.Sync()
}

/*
	日志器创建：NewLogger(l *zap.Logger) Logger

含义：将底层 zap.Logger 实例包装为自定义 zapLogger（实现 Logger 接口），使其支持自定义日志功能（如分级日志、上下文扩展）。
核心逻辑：初始化 zapLogger 结构体，内嵌 infoLogger（默认级别为 Info），绑定底层 zap.Logger。
*/
func NewLogger(l *zap.Logger) Logger {
	return &zapLogger{
		zapLogger: l,
		infoLogger: infoLogger{
			log:   l,
			level: zap.InfoLevel,
		},
	}
}

/*
底层实例获取：ZapLogger() *zap.Logger
含义：返回全局日志器底层的 zap.Logger 实例，供其他日志包装器（如 klog）集成。
作用：实现多日志系统兼容，确保第三方库的日志输出通过当前日志器统一处理。
*/
func ZapLogger() *zap.Logger {
	return logger.zapLogger
}

/*
4. 级别检查：CheckIntLevel(level int32) bool
含义：检查指定级别是否启用（供 klog 等外部日志包装器使用），返回 true 表示该级别日志可输出。
级别映射规则：
输入 level < 5 → 映射为 zapcore.InfoLevel（信息级）；
输入 level ≥ 5 → 映射为 zapcore.DebugLevel（调试级）。
实现逻辑：通过 zap.Logger.Check(lvl, "") 检查级别是否满足当前配置。
性能优化：对于需要复杂计算的日志参数（如序列化大对象），可先通过 Check 确认级别是否启用，避免无效计算。
*/
func CheckIntLevel(level int32) bool {
	var lvl zapcore.Level
	if level < 5 {
		lvl = zapcore.InfoLevel
	} else {
		lvl = zapcore.DebugLevel
	}
	checkEntry := logger.zapLogger.Check(lvl, "")
	return checkEntry != nil
}

// 接收 msg 和 zap.Field 结构化字段，直接输出日志。
func Debug(msg string, fields ...Field) {
	logger.zapLogger.Debug(msg, fields...)
}

// 接收格式化字符串和参数（如 %s/%d），通过 zap.SugaredLogger 实现 printf 风格输出。
func Debugf(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Debugf(format, v...)
}

// 接收 msg 和键值对列表（如 key1, val1, key2, val2），自动转换为结构化字段输出。
func Debugw(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Debugw(msg, keysAndValues...)
}

/*
含义：接收日志消息（msg）和结构化字段（zap.Field），直接输出对应级别的日志。
特点：
fields 是 zap.Field 类型的可变参数（如 zap.String("user_id", "u123")），用于定义结构化日志的键值对。
性能最优，适合需要高性能日志输出的场景（如高频接口）。
使用场景：已知日志字段结构，需要结构化输出（如 JSON 格式包含固定字段）。
*/
func Info(msg string, fields ...Field) {
	logger.zapLogger.Info(msg, fields...)
}

// Infof 方法输出 info 级别日志（格式化字符串形式）
func Infof(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Infof(format, v...)
}

// Infow 方法输出 info 级别日志（键值对形式）
func Infow(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Infow(msg, keysAndValues...)
}

// Warn 方法输出 warning 级别日志（结构化字段形式）
func Warn(msg string, fields ...Field) {
	logger.zapLogger.Warn(msg, fields...)
}

// Warnf 方法输出 warning 级别日志（格式化字符串形式）
func Warnf(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Warnf(format, v...)
}

// Warnw 方法输出 warning 级别日志（键值对形式）
func Warnw(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Warnw(msg, keysAndValues...)
}

// Error 方法输出 error 级别日志（结构化字段形式）
func Error(msg string, fields ...Field) {
	logger.zapLogger.Error(msg, fields...)
}

// Errorf 方法输出 error 级别日志（格式化字符串形式）
func Errorf(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Errorf(format, v...)
}

// Errorw 方法输出 error 级别日志（键值对形式）
func Errorw(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Errorw(msg, keysAndValues...)
}

// Panic 方法输出 panic 级别日志并终止程序（结构化字段形式）
func Panic(msg string, fields ...Field) {
	logger.zapLogger.Panic(msg, fields...)
}

// Panicf 方法输出 panic 级别日志并终止程序（格式化字符串形式）
func Panicf(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Panicf(format, v...)
}

// Panicw 方法输出 panic 级别日志并终止程序（键值对形式）
func Panicw(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Panicw(msg, keysAndValues...)
}

// Fatal 方法输出 fatal 级别日志并强制退出程序（结构化字段形式）
func Fatal(msg string, fields ...Field) {
	logger.zapLogger.Fatal(msg, fields...)
}

// Fatalf 方法输出 fatal 级别日志并强制退出程序（格式化字符串形式）
func Fatalf(format string, v ...interface{}) {
	logger.zapLogger.Sugar().Fatalf(format, v...)
}

// Fatalw 方法输出 fatal 级别日志并强制退出程序（键值对形式）
func Fatalw(msg string, keysAndValues ...interface{}) {
	logger.zapLogger.Sugar().Fatalw(msg, keysAndValues...)
}

// GetOptions 获取当前日志配置选项
func GetOptions() *Options {
	return options
}

// GetLogger 获取全局日志器实例
func GetLogger() *zapLogger {
	return logger
}
