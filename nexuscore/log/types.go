/*
// Copyright 2025 马晓璐 <15940995655@13..com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
log:types.go
典型的门面模式（Facade Pattern） 应用，通过包装 zap 的核心类型和工具函数，为项目提供统一的日志接口。其核心价值在于降低耦合、统一规范，并为未来的扩展或替换日志库提供灵活性。
解耦业务与底层日志库
通过将 zapcore.Field、zapcore.Level 定义为当前包的别名（type Field = zapcore.Field、type Level = zapcore.Level），并将 Zap 的工具函数（如 zap.String、zap.Int 等）重导出为当前包的变量，使业务代码只需依赖本包（package log），无需直接引用 zap 或 zapcore。这意味着未来若需替换日志库（如切换到 logrus），仅需修改本包的实现，无需改动所有业务代码中的日志操作逻辑。
统一日志接口规范
集中暴露一套标准化的日志类型（Field、Level）、级别常量（DebugLevel、InfoLevel 等）和字段创建函数（String、Int、Error 等），避免团队成员因直接使用不同日志库 API 导致的风格混乱，确保项目内日志操作的一致性。
预留扩展与定制空间
作为日志操作的中间层，本包可方便地对日志行为进行统一定制（如重写 Error 函数以标准化错误日志格式），无需侵入业务代码。同时，通过别名和重导出的方式，保留了底层日志库（Zap）的高性能和丰富功能，兼顾规范性与实用性。
*/

package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapcore.Field 是 zap 中用于表示结构化日志字段的核心类型（例如 log.String("user", "Alice") 中的字段）。
// 别名后，项目中可直接使用 log.Field 代替 zapcore.Field，统一字段类型的引用方式。
// 携带日志数据：每个 Field 代表一个键值对，用于向日志条目添加具体信息（如请求 ID、用户 ID、错误详情等）。
// 类型安全：通过 Type 字段标记值的类型（字符串、整数、布尔值等），确保日志序列化时的正确性。
// 高效处理：相比使用 map 传递字段，Field 经过优化，能减少内存分配，提升日志性能。
/*
type Field struct {
    Key       string      // 字段的键名（如 "user_id"、"error"）
    Type      FieldType   // 字段值的类型标识（用于序列化时的类型处理）
    Integer   int64       // 存储整数类型的值（int、int32、int64、uint 等）
    String    string      // 存储字符串类型的值
    Interface interface{} // 存储复杂类型的值（对象、切片、map 等）
}
*/
type Field = zapcore.Field

// zapcore.Level 是 zap 的日志级别类型，定义了从 Debug 到 Fatal 的各级别
// 这里通过别名将 zap 的级别常量暴露为当前包的变量，业务代码可直接使用 log.InfoLevel 等，无需关心底层是 zap 实现。
type Level = zapcore.Level

var (
	// DebugLevel 级别的日志通常内容繁多，在生产环境中通常会被禁用。
	DebugLevel = zapcore.DebugLevel
	// InfoLevel 是默认的日志优先级。
	InfoLevel = zapcore.InfoLevel
	// WarnLevel 级别的日志比 Info 更重要，但无需人工逐条查看。
	WarnLevel = zapcore.WarnLevel
	// ErrorLevel 级别的日志优先级高。如果应用运行正常，理论上不应产生任何错误级别的日志。
	ErrorLevel = zapcore.ErrorLevel
	// DPanicLevel 级别的日志用于标记特别重要的错误。在开发环境中，日志输出后会触发程序 panic。
	DPanicLevel = zapcore.DPanicLevel
	// PanicLevel 级别的日志在输出消息后，会触发程序 panic。
	PanicLevel = zapcore.PanicLevel
	// FatalLevel 级别的日志在输出消息后，会调用 os.Exit(1) 终止程序。
	FatalLevel = zapcore.FatalLevel
)

// Alias for zap type functions.
var (
	Any         = zap.Any
	Array       = zap.Array
	Object      = zap.Object
	Binary      = zap.Binary
	Bool        = zap.Bool
	Bools       = zap.Bools
	ByteString  = zap.ByteString
	ByteStrings = zap.ByteStrings
	Complex64   = zap.Complex64
	Complex64s  = zap.Complex64s
	Complex128  = zap.Complex128
	Complex128s = zap.Complex128s
	Duration    = zap.Duration
	Durations   = zap.Durations
	Err         = zap.Error
	Errors      = zap.Errors
	Float32     = zap.Float32
	Float32s    = zap.Float32s
	Float64     = zap.Float64
	Float64s    = zap.Float64s
	Int         = zap.Int
	Ints        = zap.Ints
	Int8        = zap.Int8
	Int8s       = zap.Int8s
	Int16       = zap.Int16
	Int16s      = zap.Int16s
	Int32       = zap.Int32
	Int32s      = zap.Int32s
	Int64       = zap.Int64
	Int64s      = zap.Int64s
	Namespace   = zap.Namespace
	Reflect     = zap.Reflect
	Stack       = zap.Stack
	String      = zap.String
	Stringer    = zap.Stringer
	Strings     = zap.Strings
	Time        = zap.Time
	Times       = zap.Times
	Uint        = zap.Uint
	Uints       = zap.Uints
	Uint8       = zap.Uint8
	Uint8s      = zap.Uint8s
	Uint16      = zap.Uint16
	Uint16s     = zap.Uint16s
	Uint32      = zap.Uint32
	Uint32s     = zap.Uint32s
	Uint64      = zap.Uint64
	Uint64s     = zap.Uint64s
	Uintptr     = zap.Uintptr
	Uintptrs    = zap.Uintptrs
)
