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
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
)

// 测试NewOptions是否返回正确的默认配置
func TestNewOptions(t *testing.T) {
	opts := NewOptions()

	if opts.Level != zapcore.InfoLevel.String() {
		t.Errorf("默认日志级别错误，期望 %s，实际 %s", zapcore.InfoLevel.String(), opts.Level)
	}

	if opts.Format != consoleFormat {
		t.Errorf("默认格式错误，期望 %s，实际 %s", consoleFormat, opts.Format)
	}

	if opts.EnableColor != false {
		t.Error("默认应关闭彩色输出")
	}

	if opts.EnableCaller != false {
		t.Error("默认应关闭调用者信息")
	}

	if len(opts.OutputPaths) != 1 || opts.OutputPaths[0] != "stdout" {
		t.Errorf("默认输出路径错误，实际 %v", opts.OutputPaths)
	}

	if len(opts.ErrorOutputPaths) != 1 || opts.ErrorOutputPaths[0] != "stderr" {
		t.Errorf("默认错误输出路径错误，实际 %v", opts.ErrorOutputPaths)
	}
}

// 测试Validate方法对各种配置的验证结果
func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Options
		wantErr bool
	}{
		{
			name: "有效配置",
			opts: &Options{
				Level:  zapcore.InfoLevel.String(),
				Format: consoleFormat,
			},
			wantErr: false,
		},
		{
			name: "JSON格式有效配置",
			opts: &Options{
				Level:  zapcore.DebugLevel.String(),
				Format: jsonFormat,
			},
			wantErr: false,
		},
		{
			name: "格式大小写不敏感",
			opts: &Options{
				Level:  zapcore.WarnLevel.String(),
				Format: "JSON",
			},
			wantErr: false,
		},
		{
			name: "无效日志级别",
			opts: &Options{
				Level:  "invalid-level",
				Format: consoleFormat,
			},
			wantErr: true,
		},
		{
			name: "无效日志格式",
			opts: &Options{
				Level:  zapcore.ErrorLevel.String(),
				Format: "xml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.opts.Validate()
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("验证结果错误，期望错误 %v，实际错误 %v", tt.wantErr, errs)
			}
		})
	}
}

// 测试AddFlags方法是否正确绑定命令行参数
func TestOptions_AddFlags(t *testing.T) {
	opts := NewOptions()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	opts.AddFlags(fs)

	// 验证是否包含所有预期的flag（兼容旧版本pflag，移除MarkedHidden判断）
	expectedFlags := []string{
		flagLevel,
		flagFormat,
		flagEnableColor,
		flagEnableCaller,
		flagOutputPaths,
		flagErrorOutputPaths,
	}

	for _, flagName := range expectedFlags {
		// 直接检查flag是否存在
		if fs.Lookup(flagName) == nil {
			t.Errorf("未找到绑定的命令行参数: %s", flagName)
		}
	}

	// 测试命令行参数解析
	args := []string{
		"--log.level=debug",
		"--log.format=json",
		"--log.enable-color=true",
		"--log.enable-caller=true",
		"--log.output-paths=stdout,file.log",
		"--log.error-output-paths=stderr,error.log",
	}

	if err := fs.Parse(args); err != nil {
		t.Fatalf("解析命令行参数失败: %v", err)
	}

	if opts.Level != "debug" {
		t.Errorf("日志级别解析错误，期望 debug，实际 %s", opts.Level)
	}
	if opts.Format != "json" {
		t.Errorf("日志格式解析错误，期望 json，实际 %s", opts.Format)
	}
	if !opts.EnableColor {
		t.Error("彩色输出应被启用")
	}
	if !opts.EnableCaller {
		t.Error("调用者信息应被启用")
	}
	if len(opts.OutputPaths) != 2 || opts.OutputPaths[1] != "file.log" {
		t.Errorf("输出路径解析错误，实际 %v", opts.OutputPaths)
	}
	if len(opts.ErrorOutputPaths) != 2 || opts.ErrorOutputPaths[1] != "error.log" {
		t.Errorf("错误输出路径解析错误，实际 %v", opts.ErrorOutputPaths)
	}
}

// 测试String方法是否正确序列化配置
func TestOptions_String(t *testing.T) {
	opts := NewOptions()
	jsonStr := opts.String()

	expectedSubstrings := []string{
		"\"level\":\"info\"",
		"\"format\":\"console\"",
		"\"enable-color\":false",
		"\"output-paths\":[\"stdout\"]",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(jsonStr, substr) {
			t.Errorf("序列化结果不包含预期内容 %s，实际: %s", substr, jsonStr)
		}
	}
}
