package options

import (
	"encoding/json"

	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/idutil"
)

// 定义Options参数结构
type Options struct {
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"   mapstructure:"server"`
	MySQLOptions            *genericoptions.MySQLOptions           `json:"mysql"    mapstructure:"mysql"`
	JwtOptions              *genericoptions.JwtOptions             `json:"jwt"      mapstructure:"jwt"`
	Log                     *log.Options                           `json:"log"      mapstructure:"log"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
}

func NewOptions() *Options {
	o := &Options{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		MySQLOptions:            genericoptions.NewMySQLOptions(),
		JwtOptions:              genericoptions.NewJwtOptions(),
		Log:                     log.NewOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
	}
	return o
}

func (o *Options) Flags() (fss cliFlag.NamedFlagSets) {
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("generic"))
	o.JwtOptions.AddFlags(fss.FlagSet("jwt"))
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))
	o.Log.AddFlags(fss.FlagSet("log"))
	return fss
}

func (o *Options) Validate() []error {
	var errs []error
	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.JwtOptions.Validate()...)
	errs = append(errs, o.MySQLOptions.Validate()...)
	return errs
}

func (o *Options) Complete() error {
	if o.JwtOptions.Key == "" {
		o.JwtOptions.Key = idutil.NewSecretKey()
	}
	return nil
}
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
