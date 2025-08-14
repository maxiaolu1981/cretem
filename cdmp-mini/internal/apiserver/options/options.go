package options

import (
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	flag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

// 定义Options参数结构
type Options struct {
	MySQLOptions            *genericoptions.MySQLOptions `json:"mysql"`
	Log                     *log.Options
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"   mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	FeatureOptions          *genericoptions.FeatureOptions         `json:"feature"  mapstructure:"feature"`
}

func NewOptions() *Options {
	return &Options{
		MySQLOptions: genericoptions.NewMySQLOptions(),
		Log:          log.NewOptions(),
	}
}

func (o *Options) Flags() (fss flag.NamedFlagSets) {
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))
	o.Log.AddFlags(fss.FlagSet("logs"))
	return
}

func (o *Options) Validate() []error {
	var errs []error
	errs = append(errs, o.MySQLOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	return errs
}
