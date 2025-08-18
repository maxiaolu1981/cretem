package options

import (
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

// 定义Options参数结构
type Options struct {
	GenericServerRunOptions *genericoptions.ServerRunOptions `json:"server"   mapstructure:"server"`
	MySQLOptions            *genericoptions.MySQLOptions     `json:"mysql"    mapstructure:"mysql"`
	JwtOptions              *genericoptions.JwtOptions       `json:"jwt"      mapstructure:"jwt"`
	Log                     *log.Options                     `json:"log"      mapstructure:"log"`
}

func NewOptions() *Options {
	o := &Options{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		MySQLOptions:            genericoptions.NewMySQLOptions(),
		JwtOptions:              genericoptions.NewJwtOptions(),
		Log:                     log.NewOptions(),
	}
	return o
}

func(o *Options) Flags() (fss flag.NamedFlagSets) {
	   o.GenericServerRunOptions.
}
