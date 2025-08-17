package options

import (
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	cliFlag "github.com/maxiaolu1981/cretem/nexuscore/component-base/cli/flag"
)

// 定义Options参数结构
type Options struct {
	GenericServerRunOptions *genericoptions.ServerRunOptions `json:"server"   mapstructure:"server"`
	//GRPCOptions             *genericoptions.//GRPCOptions            `json:"grpc"     mapstructure:"grpc"`
	InsecureServing *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	//SecureServing           *genericoptions.SecureServingOptions   `json:"secure"   mapstructure:"secure"`
	MySQLOptions *genericoptions.MySQLOptions `json:"mysql"    mapstructure:"mysql"`
	//RedisOptions            *genericoptions.RedisOptions           `json:"redis"    mapstructure:"redis"`
	JwtOptions     *genericoptions.JwtOptions     `json:"jwt"      mapstructure:"jwt"`
	Log            *log.Options                   `json:"log"      mapstructure:"log"`
	FeatureOptions *genericoptions.FeatureOptions `json:"feature"  mapstructure:"feature"`
}

func NewOptions() *Options {
	o := Options{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		//GRPCOptions:             genericoptions.NewGRPCOptions(),
		//InsecureServing:         genericoptions.NewInsecureServingOptions(),
		//SecureServing:           genericoptions.NewSecureServingOptions(),
		MySQLOptions: genericoptions.NewMySQLOptions(),
		//	RedisOptions:            genericoptions.NewRedisOptions(),
		JwtOptions: genericoptions.NewJwtOptions(),
		Log:        log.NewOptions(),
		//	FeatureOptions:          genericoptions.NewFeatureOptions(),
	}

	return &o
}

func (o *Options) Flags() (fss cliFlag.NamedFlagSets) {
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("generic"))
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
