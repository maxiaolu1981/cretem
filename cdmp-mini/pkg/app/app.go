package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	flag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/term"
	"github.com/spf13/cobra"
)

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

type RunFunc func(basename string) error
type Option func(app *App)

// 程序运行的主结构，对cobra进行进一步封装，用于构建命令行应用程序框架
type App struct {
	// 应用二进制文件名
	basename string
	// 应用名称
	name string
	// 应用描述
	description string
	// 命令行配置选项
	options CliOptions
	// 应用运行的核心函数
	runFunc RunFunc
	// 是否静默模式（不输出日志信息）
	silence bool
	// 是否禁用版本信息
	noVersion bool
	// 是否禁用配置文件
	noConfig bool
	// 子命令集合
	commands []*Command
	// 位置参数验证函数
	args cobra.PositionalArgs
	// 底层cobra命令对象
	cmd *cobra.Command
}

func WithDesriptions(desc string) Option {
	return func(app *App) {
		app.description = desc
	}
}

func WithOptions(opts options.Options) Option {
	return func(app *App) {
		app.options = opts
	}
}

func WithDefaultValidArgs() Option {
	return func(app *App) {
		app.args = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("%s不允许存在参数", cmd.Name())

			}
			return nil
		}
	}
}

func WithRunFunc(runFunc RunFunc) Option {
	return func(app *App) {
		app.runFunc = runFunc
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

func WithValidate(args cobra.PositionalArgs) Option {
	return func(app *App) {
		app.args = args
	}
}

func NewApp(basename, name string, opts ...Option) *App {
	app := &App{
		basename: basename,
		name:     name,
	}
	for _, o := range opts {
		o(app)
	}
	app.buildCommand()
	return app
}

func (a *App) buildCommand() {
	cmd := cobra.Command{
		Use:           a.basename,
		Short:         a.name,
		Long:          a.description,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          a.args,
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.Flags().SortFlags = true
	cliFlag.InitFlags(cmd.Flags())

	if len(a.commands) > 0 {
		for _, command := range a.commands {
			cmd.AddCommand(command.cobraCommand())
		}
		cmd.SetHelpCommand(helpCommand(FormatBaseName(a.basename)))
	}
	if a.runFunc != nil {
		cmd.RunE = a.runCommand()
	}
	var namedFlagSets cliFlag.NamedFlagSets
	if a.options != nil {
		namedFlagSets = options.Options.FLags()
	}

}

func addCmdTemplate(cmd *cobra.Command, namedFlagSets flag.NamedFlagSets) {
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		flag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)

		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		flag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})
}

// 检查并补充缺失的参数（如果需要）。
// 验证所有参数是否合法（比如端口号不能是负数）。
// （可选）打印最终的配置信息，让用户知道程序用了哪些参数。
