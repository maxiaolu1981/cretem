package app

import (
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

type CliOptions interface {
	Flags() cliFlag.NamedFlagSets
	Validate() []error
}

type RunFunc func(basename string) error

type Option func(app *App)

type CompleteableOptions interface {
	Complete() error
}
