package main

import (
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

func main() {
	// 1. 创建命名标志集管理器（NamedFlagSets）
	var fss flag.NamedFlagSets

	// 2. 按功能分组添加标志集
	// 2.1 全局参数分组（global）
	globalFlags := fss.FlagSet("global")
	globalFlags.BoolP("help", "h", false, "显示帮助信息")
	globalFlags.String("log-level", "info", "全局日志级别（debug/info/warn/error）")
	globalFlags.Bool("debug", false, "启用全局调试模式")

	// 2.2 服务器参数分组（server）
	serverFlags := fss.FlagSet("server")
	serverFlags.Int("port", 8080, "服务器监听端口")
	serverFlags.String("host", "0.0.0.0", "绑定的主机地址")
	serverFlags.Duration("timeout", 30, "连接超时时间（秒）")

	// 2.3 数据库参数分组（database）
	dbFlags := fss.FlagSet("database")
	dbFlags.String("db-host", "localhost", "数据库主机地址")
	dbFlags.Int("db-port", 3306, "数据库端口")
	dbFlags.String("db-name", "appdb", "数据库名称")
	dbFlags.StringP("db-user", "u", "root", "数据库用户名")
	dbFlags.StringP("db-pass", "p", "", "数据库密码（必填）")

	// 3. 解析命令行参数（以服务器参数为例）
	// 实际场景中可根据子命令选择对应的标志集解析
	if err := dbFlags.Parse(os.Args[1:]); err != nil {
		panic(err)
	}

	// 4. 打印分节帮助信息（列宽80，适合终端展示）
	// 当用户输入--help时触发
	if globalFlags.Lookup("help").Changed {
		printHelp(fss)
		os.Exit(0)
	}

	// 5. 业务逻辑：使用解析后的参数
	port, _ := serverFlags.GetInt("port")
	fmt.Printf("服务器启动成功，监听端口: %d\n", port)
}

// 打印分节帮助信息
func printHelp(fss flag.NamedFlagSets) {
	fmt.Println("用法: app [参数]")
	fmt.Println("\n命令行参数分为以下几组：")

	// 调用PrintSections按分组打印帮助信息，列宽80
	flag.PrintSections(os.Stdout, fss, 80)

	fmt.Println("\n示例:")
	fmt.Println("  app --port 9090 --db-host db.example.com --log-level debug")
}
