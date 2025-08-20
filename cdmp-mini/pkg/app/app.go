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
	commands    []Command
	command     *cobra.Command
	args        cobra.PositionalArgs
	noVersion   bool
	noConfig    bool
	silence     bool
	runFunc     RunFunc
	cmd         *cobra.Command
	options     CliOptions
}

type RunFunc func(basename string) error
type Option func(app *App)

// 程序运行的主结构，对cobra进行进一步封装，用于构建命令行应用程序框架

func WithDesriptions(desc string) Option {
	return func(app *App) {
		app.description = desc
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
		cmd.RunE = a.runCommand
	}

	var namedFlagSets cliFlag.NamedFlagSets
	if a.options != nil {
		fs := cmd.Flags()
		for _, f := range namedFlagSets.FlagSets {
			fs.AddFlagSet(f)
		}
	}
	if !a.noVersion {
		verflag.AddFlags(namedFlagSets.FlagSet("global"))
	}
	if !a.noConfig {
		addConfigFlag(a.basename, namedFlagSets.FlagSet("global"))
	}
	cmd.SetHelpCommand(helpCommand(cmd.Name()))
	addCmdTemplate(&cmd, namedFlagSets)
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
