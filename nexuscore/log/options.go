/*
package log
options.go
// Copyright 2025 马晓璐 <15940995655@13..com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
核心定位
作为日志系统的 "配置中枢"，负责定义日志的可配置项（如级别、格式、输出路径等），并提供配置的初始化、验证和外部（命令行）交互能力，为后续日志实例的创建提供标准化参数。
设计思路
结构化封装：用Options结构体集中管理所有日志配置参数，通过标签支持 JSON 序列化和配置文件映射。
默认值机制：通过NewOptions()提供开箱即用的合理默认配置（如默认级别info、输出到控制台），降低使用门槛。
合法性校验：Validate()方法确保配置参数符合预期（如日志级别必须有效、格式只能是console或json），避免运行时错误。
外部可配置：通过AddFlags()绑定命令行参数，允许用户在启动程序时动态调整日志行为（无需修改代码）。
关键功能
实现了日志配置的 "定义 - 初始化 - 验证 - 配置" 全流程，既保证了配置的规范性（通过验证），又提供了灵活性（支持命令行动态调整），为日志系统的高可配置性奠定基础。
使用场景
通常与日志实例化逻辑配合使用：先通过此模块加载并验证配置，再根据配置参数初始化 Zap 日志器（zap.Logger），最终实现按预期格式、级别和路径输出日志。
核心价值
标准化：统一日志配置的参数名、格式和验证规则，避免配置混乱。
灵活性：支持通过命令行或配置文件自定义日志行为，适应不同环境（开发 / 生产）需求。
可扩展性：结构体设计便于后续添加新的配置项（如日志轮转策略、字段过滤等）。
*/

package log

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
)

const (
	// 命令行参数名（flag）
	flagLevel            = "log.level"              // 日志级别参数名
	flagFormat           = "log.format"             // 日志格式参数名
	flagEnableColor      = "log.enable-color"       // 彩色输出参数名
	flagEnableCaller     = "log.enable-caller"      // 调用者信息参数名
	flagOutputPaths      = "log.output-paths"       // 正常日志输出路径参数名
	flagErrorOutputPaths = "log.error-output-paths" // 错误日志输出路径参数名
	// 支持的日志格式
	consoleFormat = "console" // 控制台文本格式（人类可读）
	jsonFormat    = "json"    // JSON 格式（机器可读，便于日志收集分析）
	logPath       = "/tmp/cdmp-logs/app.log"
	errorLogPath  = "/tmp/cdmp-logs/error.log"
)

/*
封装了所有日志相关的配置项：
Level：日志级别（如 info、debug、warn 等）
Format：日志输出格式（console 或 json）
EnableColor：是否启用彩色输出
EnableCaller：是否在日志中包含调用者信息
OutputPaths：正常日志的输出路径
ErrorOutputPaths：错误日志的输出路径
*/
type Options struct {
	Level            string   `json:"level" mapstructure:"level"`
	Format           string   `json:"format" mapstructure:"format"`
	EnableColor      bool     `json:"enable-color" mapstructure:"enable-color"`
	EnableCaller     bool     `json:"enable-caller" mapstructure:"enable-caller"`
	OutputPaths      []string `json:"output-paths" mapstructure:"output-paths"`
	ErrorOutputPaths []string `json:"error-output-paths" mapstructure:"error-output-paths"`
}

// 创建带有默认值的配置实例
func NewOptions() *Options {
	return &Options{
		Level:            zapcore.DebugLevel.String(),
		Format:           consoleFormat,
		EnableColor:      false,
		EnableCaller:     false,
		OutputPaths:      []string{logPath, "stdout"},
		ErrorOutputPaths: []string{errorLogPath, "stderr"},
	}
}

// 验证配置的有效性，检查日志级别和格式是否合法
func (o *Options) Validate() []error {
	var errs []error
	var zapLevel zapcore.Level
	//若级别无效（如 "invalid"），添加错误信息
	if err := zapLevel.UnmarshalText([]byte(o.Level)); err != nil {
		errs = append(errs, err)
	}

	format := strings.ToLower(o.Format)
	if format != consoleFormat && format != jsonFormat {
		errs = append(errs, fmt.Errorf("not a valid log format: %q", o.Format))
	}

	return errs
}

// 绑定命令行参数
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	// 绑定日志级别到命令行参数 --log.level
	fs.StringVar(&o.Level, flagLevel, o.Level, "Minimum log output `LEVEL`.")
	// 绑定日志格式到命令行参数 --log.format
	fs.StringVar(&o.Format, flagFormat, o.Format, "Log output `FORMAT`, support plain or json format.")
	// 绑定彩色输出开关到 --log.enable-color
	fs.BoolVar(&o.EnableColor, flagEnableColor, o.EnableColor, "Enable output ansi colors in plain format logs.")
	// 绑定调用者信息开关到 --log.enable-caller
	fs.BoolVar(&o.EnableCaller, flagEnableCaller, o.EnableCaller, "Enable output of caller information in the log.")
	// 绑定正常日志输出路径到 --log.output-paths
	fs.StringSliceVar(&o.OutputPaths, flagOutputPaths, o.OutputPaths, "Output paths of log.")
	// 绑定错误日志输出路径到 --log.error-output-paths
	fs.StringSliceVar(&o.ErrorOutputPaths, flagErrorOutputPaths, o.ErrorOutputPaths, "Error output paths of log.")
}

// 配置序列化
// ：将 Options 实例序列化为 JSON 字符串，方便打印配置信息（如调试时输出当前日志配置）。
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
