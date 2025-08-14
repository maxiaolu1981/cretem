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
/*
该包是一个基于 cobra 库的命令行应用子命令管理工具，主要功能包括：
定义Command结构体作为子命令的抽象，封装命令的用法、描述、选项、子命令及运行逻辑；
提供NewCommand及CommandOption机制，简化子命令的创建和配置；
支持命令层级结构（通过添加子命令），并自动转换为 cobra 库的Command类型以适配命令行交互；
集成错误处理（如运行函数报错时的格式化输出）和跨平台适配（如FormatBaseName处理 Windows 下的可执行文件名）。
核心目标是降低基于 cobra 开发命令行工具的复杂度，提供更简洁的 API 用于定义和管理命令。
*/

package app

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/fatih/color" // 用于终端颜色输出
	"github.com/spf13/cobra" // Go常用的命令行框架
)

// Command 是命令行应用的子命令结构。
// 建议通过 app.NewCommand() 函数创建命令实例。
type Command struct {
	usage    string         // 命令用法（如"create"，用于命令行调用）
	desc     string         // 命令描述（简短说明，用于--help输出）
	options  CliOptions     // 命令的选项配置（如命令行标志）
	commands []*Command     // 子命令列表（当前命令的下级命令）
	runFunc  RunCommandFunc // 命令运行的回调函数（执行命令时触发）
}

// CommandOption 定义初始化Command结构体的可选参数（函数式选项模式）
type CommandOption func(*Command)

// WithCommandOptions 用于为命令开启从命令行读取选项的功能（绑定CliOptions）
func WithCommandOptions(opt CliOptions) CommandOption {
	return func(c *Command) {
		c.options = opt
	}
}

// RunCommandFunc 定义命令启动时的回调函数类型，接收命令行参数并返回可能的错误
type RunCommandFunc func(args []string) error

// WithCommandRunFunc 用于设置命令的启动回调函数（命令执行时会调用该函数）
func WithCommandRunFunc(run RunCommandFunc) CommandOption {
	return func(c *Command) {
		c.runFunc = run
	}
}

// NewCommand 基于给定的命令用法、描述和其他选项，创建一个新的子命令实例
func NewCommand(usage string, desc string, opts ...CommandOption) *Command {
	c := &Command{
		usage: usage,
		desc:  desc,
	}

	// 应用所有可选配置
	for _, o := range opts {
		o(c)
	}

	return c
}

// AddCommand 为当前命令添加一个子命令
func (c *Command) AddCommand(cmd *Command) {
	c.commands = append(c.commands, cmd)
}

// AddCommands 为当前命令添加多个子命令
func (c *Command) AddCommands(cmds ...*Command) {
	c.commands = append(c.commands, cmds...)
}

// cobraCommand 将当前Command转换为cobra库的Command类型（适配cobra框架）
func (c *Command) cobraCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   c.usage, // 映射自定义Command的usage到cobra的Use
		Short: c.desc,  // 映射自定义Command的desc到cobra的Short
	}
	cmd.SetOutput(os.Stdout)      // 设置命令输出到标准输出
	cmd.Flags().SortFlags = false // 不排序标志（保持定义顺序）

	// 递归添加所有子命令（将子Command也转换为cobra.Command）
	if len(c.commands) > 0 {
		for _, command := range c.commands {
			cmd.AddCommand(command.cobraCommand())
		}
	}

	// 绑定运行函数：若设置了runFunc，关联到cobra.Command.Run
	if c.runFunc != nil {
		cmd.Run = c.runCommand
	}

	// 绑定命令选项：将CliOptions中的标志添加到cobra命令的标志集
	if c.options != nil {
		for _, f := range c.options.Flags().FlagSets {
			cmd.Flags().AddFlagSet(f)
		}
	}

	// 添加帮助命令标志（如--help）
	addHelpCommandFlag(c.usage, cmd.Flags())

	return cmd
}

// runCommand 是cobra.Command.Run的适配方法，用于触发自定义的runFunc
func (c *Command) runCommand(cmd *cobra.Command, args []string) {
	if c.runFunc != nil {
		// 执行自定义运行函数，若出错则以红色输出错误并退出
		if err := c.runFunc(args); err != nil {
			fmt.Printf("%v %v\n", color.RedString("错误:"), err)
			os.Exit(1)
		}
	}
}

// AddCommand 为应用（App）添加一个子命令
func (a *App) AddCommand(cmd *Command) {
	a.commands = append(a.commands, cmd)
}

// AddCommands 为应用（App）添加多个子命令
func (a *App) AddCommands(cmds ...*Command) {
	a.commands = append(a.commands, cmds...)
}

// FormatBaseName 根据给定的名称，按不同操作系统格式化为可执行文件名
func FormatBaseName(basename string) string {
	// 在Windows系统下，忽略大小写并移除.exe后缀（统一处理可执行文件名）
	if runtime.GOOS == "windows" {
		basename = strings.ToLower(basename)
		basename = strings.TrimSuffix(basename, ".exe")
	}

	return basename
}
