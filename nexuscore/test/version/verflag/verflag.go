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

package main

import (
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version/verflag"
	"github.com/spf13/pflag"
)

// 自定义业务参数
type AppConfig struct {
	Port    int    // 服务端口
	LogPath string // 日志路径
}

func main() {
	// 1. 初始化业务参数标志集
	cfg := &AppConfig{}
	fs := pflag.NewFlagSet("myapp", pflag.ExitOnError)

	// 添加业务参数
	fs.IntVar(&cfg.Port, "port", 8080, "服务监听端口")
	fs.StringVar(&cfg.LogPath, "log-path", "./logs", "日志存储路径")

	// 2. 将版本标志添加到业务标志集（关键：使--version对当前程序生效）
	verflag.AddFlags(fs)

	// 3. 解析命令行参数
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Printf("解析命令行参数失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 检查是否需要打印版本信息并退出（核心函数）
	// 若用户传入--version或--version=raw，会在此处打印版本并退出
	verflag.PrintAndExitIfRequested()

	// 5. 正常业务逻辑（若未触发版本打印，则继续执行）
	fmt.Printf("服务启动成功！端口: %d, 日志路径: %s\n", cfg.Port, cfg.LogPath)
	fmt.Printf("当前版本: %s\n", version.Get().GitVersion)
}
