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
