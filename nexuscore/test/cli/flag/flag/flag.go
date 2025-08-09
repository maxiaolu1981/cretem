package main

import (
	goflag "flag" // 标准库flag重命名为goflag
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag" // 自定义flag包
	"github.com/spf13/pflag"
)

// 初始化日志配置
func init() {
	//log.SetLevel(log.DebugLevel) // 假设日志库支持该方法
}

func main() {
	// 1. 创建pflag标志集
	fs := pflag.NewFlagSet("myapp", pflag.ExitOnError)

	// 2. 定义pflag标志
	var (
		logLevel string
		debug    bool
		port     int
		username string
	)

	fs.StringVar(&logLevel, "log-level", "info", "日志级别")
	fs.BoolVar(&debug, "debug", false, "启用调试模式")
	fs.IntVar(&port, "server_port", 8080, "服务器端口")
	fs.StringVar(&username, "user_name", "admin", "用户名")

	// 3. 定义标准库flag标志（使用goflag）
	var timeout int
	goflag.IntVar(&timeout, "timeout", 30, "超时时间")

	// 4. 初始化标志集（自定义flag包的InitFlags）
	flag.InitFlags(fs)

	// 5. 可选：启用下划线警告
	// fs.SetNormalizeFunc(flag.WarnWordSepNormalizeFunc)

	// 6. 解析参数
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Printf("参数解析失败: %v\n", err)
		os.Exit(1)
	}

	// 7. 打印标志（自定义flag包的PrintFlags）
	flag.PrintFlags(fs)

	// 8. 业务逻辑
	fmt.Println("\n=== 应用配置 ===")
	fmt.Printf("日志级别: %s\n", logLevel)
	fmt.Printf("调试模式: %v\n", debug)
	fmt.Printf("服务器端口: %d\n", port)
	fmt.Printf("用户名: %s\n", username)
	fmt.Printf("超时时间: %d秒\n", timeout)
}
