package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag/globalflag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/term"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version/verflag"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

type App struct {
	basename   string
	name       string
	desription string
	options    CliOptions
	noVersion  bool
	noConfig   bool
	silence    bool
	cmd        *cobra.Command
	runFunc    RunFunc
	args       cobra.PositionalArgs
}

func WithDescription(description string) Option {
	return func(app *App) {
		app.desription = description
	}
}
func WithOptions(opts *options.Options) Option {
	return func(app *App) {
		app.options = opts
	}
}

func WithDefaultValidArgs() Option {
	return func(app *App) {

		app.args = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("不允许输入任何参数.")
			}
			return nil
		}
	}
}

func WithNoVersion() Option {
	return func(app *App) {
		app.noVersion = true
	}
}

func WithValidArgs(args cobra.PositionalArgs) Option {
	return func(app *App) {
		app.args = args
	}
}

func WithNoConfig() Option {
	return func(app *App) {
		app.noConfig = true
	}
}

func WithNoSilence() Option {
	return func(app *App) {
		app.silence = true
	}
}

func WithRunFunc(runFunc RunFunc) Option {
	return func(app *App) {
		app.runFunc = runFunc
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

func (app *App) buildCommand() {
	cmd := &cobra.Command{
		Use:           app.basename,
		Short:         app.name,
		Long:          app.desription,
		Args:          app.args,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.Flags().SortFlags = true
	cliFlag.InitFlags(cmd.Flags())

	var namedFlagSets cliFlag.NamedFlagSets
	if app.options != nil {
		namedFlagSets = app.options.Flags()
		fs := cmd.Flags()
		for _, f := range namedFlagSets.FlagSets {
			fs.AddFlagSet(f)
		}
	}
	if !app.noVersion {
		verflag.AddFlags(namedFlagSets.FlagSet("global"))

	}
	if !app.noConfig {
		addConfigFlag(app.basename, namedFlagSets.FlagSet("global"))
	}

	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())

	addCmdTemplate(cmd, &namedFlagSets)

	if app.runFunc != nil {
		cmd.RunE = app.runCommand
	}
	app.cmd = cmd
}

func addCmdTemplate(cmd *cobra.Command, fss *cliFlag.NamedFlagSets) {
	col, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetHelpFunc(func(c *cobra.Command, s []string) {
		cliFlag.PrintSections(cmd.OutOrStdout(), *fss, col)
	})
}

func (app *App) runCommand(cmd *cobra.Command, args []string) error {
	printWorkingDir()
	cliFlag.PrintFlags(cmd.Flags())

	if !app.noConfig {
		err := viper.BindPFlags(cmd.Flags())
		if err != nil {
			return err
		}
		err = viper.Unmarshal(app.options)
		if err != nil {
			return err
		}
	}

	if !app.silence {
		log.Infof("%v 开始 %s....", progressMessage, app.name)
		if !app.noConfig {
			printConfig()
		}
		if !app.noVersion {
			version.Get().PrintVersionWithLog()
		}

		fmt.Printf("%v打印Options....%s", progressMessage, app.options)

	}

	if app.options != nil {
		if err := app.applyOptionRules(); err != nil {
			return err
		}
	}

	if app.runFunc != nil {
		app.runFunc(app.basename)
	}
	return nil
}

func (app *App) applyOptionRules() error {
	if completeableOptions, ok := app.options.(CompleteableOptions); ok {
		if err := completeableOptions.Complete(); err != nil {
			return err
		}
	}
	if errs := app.options.Validate(); errs != nil {
		return errors.NewAggregate(errs)
	}
	return nil
}

func printWorkingDir() {
	pwd, _ := os.Getwd()
	log.Infof("%v当前工作目录:%s", progressMessage, pwd)
}

func (app *App) Run() error {
	if err := app.cmd.Execute(); err != nil {
		fmt.Printf("%v %v", color.RedString("错误="), err.Error())
		os.Exit(1)
	}
	return nil
}
