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

// Package app 提供基于cobra和viper的CLI应用框架，用于快速构建具有子命令、配置管理、版本控制等功能的命令行工具。
// 核心功能包括：
// 1. 应用生命周期管理（初始化、启动、命令执行）；
// 2. 命令行参数分组与解析（基于NamedFlagSets）；
// 3. 配置文件处理（通过viper绑定参数与配置）；
// 4. 版本信息与全局标志管理；
// 5. 彩色输出与格式化帮助信息。
// 适用于需要构建复杂命令行工具的场景，支持参数验证、配置补全、多子命令等高级特性。
// app 包定义了 CLI 应用程序的核心结构和启动逻辑，封装了命令行参数解析、配置加载、版本信息展示等功能，
// 提供了灵活的选项配置机制，简化了应用程序的初始化和运行流程。
package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	cliflag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag/globalflag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/term"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version/verflag"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	progressMessage = color.GreenString("==>") // 进度信息前缀（绿色）

	// usageTemplate 定义命令行帮助信息的格式化模板，使用彩色输出增强可读性
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
		color.CyanString("Usage:"),                        // 用法标题（青色）
		color.GreenString("{{.UseLine}}"),                 // 命令用法（绿色）
		color.GreenString("{{.CommandPath}} [command]"),   // 子命令用法（绿色）
		color.CyanString("Aliases:"),                      // 别名标题（青色）
		color.CyanString("Examples:"),                     // 示例标题（青色）
		color.CyanString("Available Commands:"),           // 可用命令标题（青色）
		color.GreenString("{{rpad .Name .NamePadding }}"), // 命令名称（绿色）
		color.CyanString("Flags:"),                        // 标志标题（青色）
		color.CyanString("Global Flags:"),                 // 全局标志标题（青色）
		color.CyanString("Additional help topics:"),       // 额外帮助主题标题（青色）
		color.GreenString("{{.CommandPath}} [command]"),   // 命令帮助提示（绿色）
	)
)

// App 是 CLI 应用程序的主结构，推荐通过 app.NewApp() 函数创建实例
type App struct {
	basename    string               // 二进制文件名（如可执行文件名称）
	name        string               // 应用程序名称
	description string               // 应用程序描述
	options     CliOptions           // 命令行选项配置
	runFunc     RunFunc              // 应用程序启动回调函数
	silence     bool                 // 是否启用静默模式（不打印启动信息）
	noVersion   bool                 // 是否不提供版本标志
	noConfig    bool                 // 是否不提供配置文件标志
	commands    []*Command           // 子命令列表
	args        cobra.PositionalArgs // 位置参数验证函数
	cmd         *cobra.Command       // cobra 命令实例（核心命令对象）
}

// Option 定义初始化应用程序结构的可选参数
type Option func(*App)

// WithOptions 启用应用程序从命令行或配置文件读取参数的功能
func WithOptions(opt CliOptions) Option {
	return func(a *App) {
		a.options = opt
	}
}

// RunFunc 定义应用程序的启动回调函数
type RunFunc func(basename string) error

// WithRunFunc 用于设置应用程序启动回调函数选项
func WithRunFunc(run RunFunc) Option {
	return func(a *App) {
		a.runFunc = run
	}
}

// WithDescription 用于设置应用程序的描述信息
func WithDescription(desc string) Option {
	return func(a *App) {
		a.description = desc
	}
}

// WithSilence 设置应用程序为静默模式，不在控制台打印启动信息、配置信息和版本信息
func WithSilence() Option {
	return func(a *App) {
		a.silence = true
	}
}

// WithNoVersion 设置应用程序不提供版本标志
func WithNoVersion() Option {
	return func(a *App) {
		a.noVersion = true
	}
}

// WithNoConfig 设置应用程序不提供配置文件标志
func WithNoConfig() Option {
	return func(a *App) {
		a.noConfig = true
	}
}

// WithValidArgs 设置验证非标志参数的函数
func WithValidArgs(args cobra.PositionalArgs) Option {
	return func(a *App) {
		a.args = args
	}
}

// WithDefaultValidArgs 设置默认的非标志参数验证函数（禁止传入任何参数）
func WithDefaultValidArgs() Option {
	return func(a *App) {
		a.args = func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q 不接受任何参数，但传入了 %q", cmd.CommandPath(), args)
				}
			}
			return nil
		}
	}
}

// NewApp 基于给定的应用名称、二进制名称和其他选项创建一个新的应用实例
func NewApp(name string, basename string, opts ...Option) *App {
	a := &App{
		name:     name,
		basename: basename,
	}

	// 应用所有选项
	for _, o := range opts {
		o(a)
	}

	// 构建命令结构
	a.buildCommand()

	return a
}

// buildCommand 构建 cobra 命令实例，配置命令行参数、子命令等
func (a *App) buildCommand() {
	cmd := cobra.Command{
		Use:           FormatBaseName(a.basename), // 命令使用格式
		Short:         a.name,                     // 命令简短描述
		Long:          a.description,              // 命令详细描述
		SilenceUsage:  true,                       // 命令出错时不打印用法
		SilenceErrors: true,                       // 不自动打印错误（由应用自行处理）
		Args:          a.args,                     // 位置参数验证函数
	}
	cmd.SetOut(os.Stdout)          // 标准输出
	cmd.SetErr(os.Stderr)          // 错误输出
	cmd.Flags().SortFlags = true   // 标志按名称排序
	cliflag.InitFlags(cmd.Flags()) // 初始化标志

	// 添加子命令
	if len(a.commands) > 0 {
		for _, command := range a.commands {
			cmd.AddCommand(command.cobraCommand())
		}
		cmd.SetHelpCommand(helpCommand(FormatBaseName(a.basename))) // 自定义帮助命令
	}

	// 设置启动函数
	if a.runFunc != nil {
		cmd.RunE = a.runCommand
	}

	// 处理命令行选项标志
	var namedFlagSets cliflag.NamedFlagSets
	if a.options != nil {
		namedFlagSets = a.options.Flags() // 获取选项定义的标志集
		fs := cmd.Flags()
		// 将所有标志集添加到命令
		for _, f := range namedFlagSets.FlagSets {
			fs.AddFlagSet(f)
		}
	}

	// 添加版本标志（如果启用）
	if !a.noVersion {
		verflag.AddFlags(namedFlagSets.FlagSet("global"))
	}

	// 添加配置文件标志（如果启用）
	if !a.noConfig {
		addConfigFlag(a.basename, namedFlagSets.FlagSet("global"))
	}

	// 添加全局标志
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())
	// 将全局标志集添加到命令
	cmd.Flags().AddFlagSet(namedFlagSets.FlagSet("global"))

	// 设置命令模板
	addCmdTemplate(&cmd, namedFlagSets)
	a.cmd = &cmd
}

// Run 用于启动应用程序
func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		fmt.Printf("%v %v\n", color.RedString("Error:"), err) // 红色错误提示
		os.Exit(1)
	}
}

// Command 返回应用程序内部的 cobra 命令实例
func (a *App) Command() *cobra.Command {
	return a.cmd
}

// runCommand 是 cobra 命令的执行函数，处理应用启动前的准备工作
func (a *App) runCommand(cmd *cobra.Command, args []string) error {
	printWorkingDir()               // 打印工作目录
	cliflag.PrintFlags(cmd.Flags()) // 打印所有标志配置

	// 处理版本信息展示
	if !a.noVersion {
		verflag.PrintAndExitIfRequested() // 如果指定了版本标志，打印版本并退出
	}

	// 处理配置文件绑定
	if !a.noConfig {
		if err := viper.BindPFlags(cmd.Flags()); err != nil { // 绑定命令行标志到 viper
			return err
		}
		if err := viper.Unmarshal(a.options); err != nil { // 将配置解析到选项结构
			return err
		}
	}

	// 打印启动信息（非静默模式）
	if !a.silence {
		log.Infof("%v Starting %s ...", progressMessage, a.name)
		if !a.noVersion {
			log.Infof("%v Version: `%s`", progressMessage, version.Get().ToJSON()) // 打印版本信息
		}
		if !a.noConfig {
			log.Infof("%v Config file used: `%s`", progressMessage, viper.ConfigFileUsed()) // 打印使用的配置文件
		}
	}

	// 应用选项规则（补全、验证、打印）
	if a.options != nil {
		if err := a.applyOptionRules(); err != nil {
			return err
		}
	}

	// 执行应用启动函数
	if a.runFunc != nil {
		return a.runFunc(a.basename)
	}

	return nil
}

// applyOptionRules 应用选项的补全、验证和打印规则
func (a *App) applyOptionRules() error {
	// 补全选项（如果支持）
	if completeableOptions, ok := a.options.(CompleteableOptions); ok {
		if err := completeableOptions.Complete(); err != nil {
			return err
		}
	}

	// 验证选项合法性
	if errs := a.options.Validate(); len(errs) != 0 {
		return errors.NewAggregate(errs) // 聚合所有错误
	}

	// 打印选项配置（如果支持且非静默模式）
	if printableOptions, ok := a.options.(PrintableOptions); ok && !a.silence {
		log.Infof("%v Config: `%s`", progressMessage, printableOptions.String())
	}

	return nil
}

// printWorkingDir 打印当前工作目录
func printWorkingDir() {
	wd, _ := os.Getwd()
	log.Infof("%v WorkingDir: %s", progressMessage, wd)
}

// addCmdTemplate 设置命令的用法和帮助模板，整合标志集信息
func addCmdTemplate(cmd *cobra.Command, namedFlagSets cliflag.NamedFlagSets) {
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout()) // 获取终端宽度

	// 自定义用法函数
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols) // 按终端宽度打印标志 sections
		return nil
	})

	// 自定义帮助函数
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols) // 按终端宽度打印标志 sections
	})
}
