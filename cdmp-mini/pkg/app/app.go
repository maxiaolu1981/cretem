package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	flag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
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
	basename    string
	name        string
	description string
	options     CliOptions
	runFunc     RunFunc
	commands    []*Command
	args        cobra.PositionalArgs
	cmd         *cobra.Command
	silence     bool
	noVersion   bool
	noConfig    bool
}

type RunFunc func(basename string) error
type Option func(app *App)

func WithDescription(description string) Option {
	return func(a *App) {
		a.description = description
	}
}

func WithOptions(opt *options.Options) Option {
	return func(a *App) {
		a.options = opt
	}
}

func WithRunFunc(runFunc RunFunc) Option {
	return func(a *App) {
		a.runFunc = runFunc
	}
}

func WithSilence() Option {
	return func(a *App) {
		a.silence = true
	}
}

func WithNoVersion() Option {
	return func(a *App) {
		a.noVersion = true
	}
}

func WithNoConfig() Option {
	return func(a *App) {
		a.noConfig = true
	}
}

func WithArgs(args cobra.PositionalArgs) Option {
	return func(a *App) {
		a.args = args
	}
}

func WithDefaultValidArgs() Option {
	return func(a *App) {
		a.args = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
			}
			return nil
		}
	}
}

func NewApp(basename string, name string, opts ...Option) *App {
	app := &App{
		basename: basename,
		name:     "api server",
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
		Example:       example,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          a.args,
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	if a.runFunc != nil {
		cmd.RunE = a.runCommand
	}

	var namedFlagSets cliFlag.NamedFlagSets
	if a.options != nil {
		namedFlagSets = a.options.Flags()
		fs := cmd.Flags()
		for _, f := range namedFlagSets.FlagSets {
			fs.AddFlagSet(f)
		}

	}

	if !a.noVersion {
		verflag.AddFlags(namedFlagSets.FlagSet("global"))
		verflag.PrintAndExitIfRequested()
	}

	a.cmd = &cmd
}

func (a *App) runCommand(cmd *cobra.Command, args []string) error {
	printWorkingDir()
	cliFlag.PrintFlags(cmd.Flags())
	if !a.noVersion {
		verflag.PrintAndExitIfRequested()
	}
	if !a.noVersion {
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			return err
		}
		if err := viper.Unmarshal(a.options); err != nil {
			return err
		}
	}
	if !a.silence {
		log.Infof("%vStarting %s", progressMessage, a.name)
		if !a.noVersion {
			log.Infof("%vVersion:%s", progressMessage, version.Get().ToJSON())
		}
		if !a.noConfig {
			log.Infof("%v Config file used:%s", progressMessage, viper.ConfigFileUsed())
		}
		if a.options != nil {
			if err := a.applyOptionRules(); err != nil {
				return err
			}
		}

	}
	if a.runFunc != nil {
		return a.runFunc(a.basename)
	}

	return nil
}

func (a *App) applyOptionRules() error {
	if completeableOptions, ok := a.options.(CompleteableOptions); ok {
		completeableOptions.Complete()
	}
	if errs := a.options.Validate(); len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	if printableOptions, ok := a.options.(PrintableOptions); ok && !a.silence {
		log.Infof("%v Config:%s", progressMessage, printableOptions.String())
	}
	return nil
}

func printWorkingDir() {
	wd, _ := os.Getwd()
	log.Infof("%v当前工作目录:%s", progressMessage, wd)
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

func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		fmt.Printf("%v %v", color.RedString("Error="), err)
		os.Exit(1)
	}
}
