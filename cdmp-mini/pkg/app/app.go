/*
这是一个功能强大的 Go 语言命令行应用程序框架包，提供了完整的 CLI 应用构建和管理功能。以下是详细分析：

核心结构
App 结构体
应用程序主结构，包含：

basename - 基础命令名称

name - 应用名称

description - 应用描述

options - 命令行选项接口

noVersion - 是否禁用版本显示

noConfig - 是否禁用配置

silence - 是否静默模式

cmd - cobra.Command 实例

runFunc - 运行函数

args - 参数验证函数

设计模式
选项模式 (Functional Options)
通过 Option 函数类型实现灵活的配置：

go
type Option func(app *App)

// 各种配置选项函数
WithDescription(), WithOptions(), WithRunFunc(), etc.
主要功能
1. 命令行构建 (buildCommand)
创建 cobra.Command 实例

设置使用说明和帮助模板

添加标志集和配置选项

设置自定义帮助函数

2. 命令运行 (runCommand)
完整的命令执行流程：

打印工作目录

显示命令行标志

绑定 Viper 配置

应用选项规则

执行运行函数

3. 选项验证 (applyOptionRules)
两级验证机制：

Complete() - 完成配置（如果实现 CompleteableOptions）

# Validate() - 验证配置有效性

特色功能
自定义帮助模板
go
var usageTemplate = fmt.Sprintf(...) // 彩色输出的帮助模板
配置管理
Viper 集成：支持多种配置源绑定

标志排序：自动排序命令行标志

命名标志集：分组管理相关标志

版本管理
go
verflag.AddFlags() // 添加版本标志
version.Get().PrintVersionWithLog() // 打印版本信息
使用示例
创建应用
go
app := NewApp("iam-apiserver", "IAM API Server",

	WithDescription("身份访问管理API服务器"),
	WithOptions(options),
	WithRunFunc(runServer),
	WithDefaultValidArgs(),

)
运行应用
go

	if err := app.Run(); err != nil {
	    // 错误处理
	}

配置选项接口
CliOptions 接口
应用选项需要实现的接口：

go

	type CliOptions interface {
	    Flags() cliFlag.NamedFlagSets
	    Validate() []error
	}

CompleteableOptions 接口（可选）
go

	type CompleteableOptions interface {
	    Complete() error
	}

输出美化
彩色输出
使用 fatih/color 包提供彩色输出：

progressMessage - 绿色进度提示

错误信息 - 红色高亮

帮助信息 - 青色标题

结构化输出
分章节显示帮助信息

自动适应终端宽度

清晰的标志分组

错误处理
聚合错误
使用 errors.NewAggregate() 处理多个验证错误

优雅退出
详细的错误信息输出

适当的退出码设置

设计优势
高度可配置：通过选项模式灵活定制应用行为

生产就绪：包含完整的配置验证和管理

用户体验：精美的帮助信息和彩色输出

扩展性强：清晰的接口设计，易于添加新功能

标准化：遵循 Go 语言和 cobra 的最佳实践

这个包提供了一个企业级的命令行应用程序框架，特别适合构建复杂的微服务和管理工具，具有完善的配置管理、错误处理和用户交互功能。

开启新对话
*/
package app

import (
	"fmt"

	"github.com/fatih/color"
)

const example = `  # 基础启动（使用默认配置）
  iam-apiserver

  # 配置MySQL和Redis连接
  iam-apiserver --mysql-host=127.0.0.1 --mysql-port=3306 --redis-url=redis://localhost:6379

  # 启用安全服务并设置GRPC端口
  iam-apiserver --secure-serving-bind-port=443 --grpc-port=9000

  # 调整日志级别并启用特定功能
  iam-apiserver --logs-level=debug --features=enable-beta

  # 查看所有配置项（按分组展示）
  iam-server --help`

var (
	progressMessage = color.GreenString("==>")

	usageTemplate = fmt.Sprintf(`%s{{if .Runnable}}
  %s{{end}}{{if .HasAvailableSubCommands}}
  %s{{end}}{{if gt (len .Aliases) 0}}

%s
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

%s
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

%s{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  %s {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

%s
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

%s
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

%s{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "%s --help" for more information about a command.{{end}}
`,
		color.CyanString("Usage:"),
		color.GreenString("{{.UseLine}}"),
		color.GreenString("{{.CommandPath}} [command]"),
		color.CyanString("Aliases:"),
		color.CyanString("Examples:"),
		color.CyanString("Available Commands:"),
		color.GreenString("{{rpad .Name .NamePadding }}"),
		color.CyanString("Flags:"),
		color.CyanString("Global Flags:"),
		color.CyanString("Additional help topics:"),
		color.GreenString("{{.CommandPath}} [command]"),
	)
)
