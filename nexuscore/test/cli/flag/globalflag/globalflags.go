package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag/globalflag"
	"github.com/spf13/pflag"
)

// 模拟全局标志（通常在基础库中定义）
func init() {
	// 用标准库flag定义全局标志（如日志级别、调试模式）
	flag.String("log_level", "info", "全局日志级别（标准库flag定义）")
	flag.Bool("debug", false, "启用调试模式（标准库flag定义）")
}

// 组件A的标志集（模拟多组件场景）
func newComponentAFlags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("component-a", pflag.ExitOnError)

	// 1. 添加全局帮助标志（--help/-h）
	globalflag.AddGlobalFlags(fs, "component-a")

	// 2. 注册需要用到的全局标志（将标准库flag关联到本地pflag）
	globalflag.Register(fs, "log_level") // 关联全局日志级别
	globalflag.Register(fs, "debug")     // 关联全局调试模式

	// 3. 定义组件A专属标志
	fs.Int("port", 8080, "组件A监听端口")
	fs.String("mode", "normal", "组件A运行模式")

	return fs
}

// 组件B的标志集（模拟多组件场景）
func newComponentBFlags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("component-b", pflag.ExitOnError)

	// 1. 添加全局帮助标志
	globalflag.AddGlobalFlags(fs, "component-b")

	// 2. 注册需要用到的全局标志（仅关联需要的）
	globalflag.Register(fs, "log_level") // 组件B也需要日志级别

	// 3. 定义组件B专属标志
	fs.String("output", "./output", "组件B输出目录")
	fs.Int("retries", 3, "组件B重试次数")

	return fs
}

func main() {
	// 解析命令行参数（模拟多组件调用）
	if len(os.Args) < 2 {
		fmt.Println("用法: ./app <component> [args...]\n组件: component-a, component-b")
		os.Exit(1)
	}

	component := os.Args[1]
	switch component {
	case "component-a":
		runComponentA(os.Args[2:])
	case "component-b":
		runComponentB(os.Args[2:])
	default:
		fmt.Printf("未知组件: %s\n", component)
		os.Exit(1)
	}
}

// 运行组件A
func runComponentA(args []string) {
	fs := newComponentAFlags()

	// 解析组件A的参数
	if err := fs.Parse(args); err != nil {
		fmt.Printf("组件A参数解析失败: %v\n", err)
		os.Exit(1)
	}

	// 演示：读取全局标志和组件专属标志
	logLevel, _ := fs.GetString("log-level") // 注意标准化后的名称（log_level→log-level）
	debug, _ := fs.GetBool("debug")
	port, _ := fs.GetInt("port")

	fmt.Println("=== 组件A启动 ===")
	fmt.Printf("全局日志级别: %s\n", logLevel)
	fmt.Printf("调试模式: %v\n", debug)
	fmt.Printf("监听端口: %d\n", port)
}

// 运行组件B
func runComponentB(args []string) {
	fs := newComponentBFlags()

	// 解析组件B的参数
	if err := fs.Parse(args); err != nil {
		fmt.Printf("组件B参数解析失败: %v\n", err)
		os.Exit(1)
	}

	// 演示：读取全局标志和组件专属标志
	logLevel, _ := fs.GetString("log-level") // 标准化后的名称
	output, _ := fs.GetString("output")
	retries, _ := fs.GetInt("retries")

	fmt.Println("=== 组件B启动 ===")
	fmt.Printf("全局日志级别: %s\n", logLevel)
	fmt.Printf("输出目录: %s\n", output)
	fmt.Printf("重试次数: %d\n", retries)
}
