package app

import (
	"os"
	"runtime"
	"strings"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// 定义cobra子命令结构体
type Command struct {
	usage    string
	desc     string
	options  CliOptions
	commands []*Command
	runFunc  RunCommandFunc
}

type RunCommandFunc func(args []string) error

func FormatBaseName(name string) string {
	name = strings.ToLower(name)
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, ".exe")
	}
	return name
}

func (c *Command) cobraCommand() *cobra.Command{
	cmd := &cobra.Command{
		Use: c.usage,
		Short: c.desc,
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.Flags().SortFlags = false
	if len(c.commands) > 0 {
		for _,command := range c.commands{
			cmd.AddCommand(command.cobraCommand())
		}
	}
	
	if c.runFunc != nil{
		cmd.Run = c.runCommand
	}

	if c.options != nil{
		for _,f := range c.options.Flags().FlagSets{
			cmd.Flags().AddFlagSet(f)
		}
	}
	addHelpCommandFlag(c.usage,cmd.Flags())

}

func(c *Command) runCommand(cmd *cobra.Command,args []string){
		if c.runFunc != nil{
			if err := c.runFunc(args); err != nil{
				fmt.Printf("%v %v\m",color.RedString("Error:"),err)
				os.Exit(1)
			}
		}
}