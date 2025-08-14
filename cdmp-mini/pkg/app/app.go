package app

import (
	"fmt"
	"os"

	flag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/spf13/cobra"
)

//程序运行的主结构,构建命令行应用程序的框架,对cobra进一步封装.

type App struct {
	basename    string
	name        string
	description string
	options     CliOptions
	runFunc     RunFunc
	silence     bool
	noVersion   bool
	noConfig    bool
	commands    []*Command
	args        cobra.PositionalArgs
	cmd         *cobra.Command
}

type RunFunc func(basename string) error

type Option func(*App)

func WithOptions(option CliOptions) Option {
	return func(app *App) {
		app.options = option
	}
}

func WithDescription(description string) Option {
	return func(app *App) {
		app.description = description
	}
}

func WithRunFunc(run RunFunc) Option {
	return func(app *App) {
		app.runFunc = run
	}
}

func WithSilence() Option {
	return func(app *App) {
		app.silence = true
	}
}

func WithNoVersion() Option {
	return func(app *App) {
		app.noVersion = true
	}
}

func WithNoConfig() Option {
	return func(app *App) {
		app.noConfig = true
	}
}

func WithValidArgs(args cobra.PositionalArgs) Option {
	return func(app *App) {
		app.args = args
	}
}

func WithDefaultValidArgs() Option {
	return func(app *App) {
		app.args = func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q不允许存在任何参数,但是发现了%q", cmd.CommandPath(), arg)
				}
			}
			return nil
		}
	}
}

func NewApp(basename, name string, opts ...Option) *App {
	a := &App{
		basename: basename,
		name:     name,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.buildCommand()
}

func (a *App) buildCommand() {
	//创建cobra.Command
	cmd := cobra.Command{
		Use:           FormatBaseName(a.basename),
		Short:         a.name,
		Long:          a.description,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          a.args,
	}
	//设置标准输出
	cmd.SetOut(os.Stdout)
	//设置标准错误输出
	cmd.SetErr(os.Stderr)
	//设置flags排序
	cmd.Flags().SortFlags = true
	//初始化flag标志 _替换为-,go flag纳入到cobra
	flag.InitFlags(cmd.Flags())
	//判断commands中是否有子命令,如果有,添加
	if len(a.commands) > 0 {
		for _, command := range a.commands {
			//	cmd.AddCommand(command)
		}
	}

	a.cmd = &cmd
}
