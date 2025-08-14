package app

import (
	flag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

type CliOptions interface {
	Flags() (fss flag.NamedFlagSets)
	Validate() []error
}
