package options

import (
	"fmt"
	"strings"

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
	if o == nil {
		return "<nil>"
	}

	var buf strings.Builder
	buf.WriteString("SERVER CONFIGURATION:\n")
	buf.WriteString("=====================\n\n")

	// GenericServerRunOptions
	buf.WriteString("GENERIC SERVER OPTIONS:\n")
	buf.WriteString("------------------------\n")
	if o.GenericServerRunOptions != nil {
		opts := o.GenericServerRunOptions
		buf.WriteString(fmt.Sprintf("  Mode: %s\n", opts.Mode))
		buf.WriteString(fmt.Sprintf("  Healthz: %t\n", opts.Healthz))
		buf.WriteString(fmt.Sprintf("  Middlewares: %v\n", opts.Middlewares))
	} else {
		buf.WriteString("  <nil>\n")
	}
	buf.WriteString("\n")

	// MySQLOptions
	buf.WriteString("MYSQL DATABASE OPTIONS:\n")
	buf.WriteString("-----------------------\n")
	if o.MySQLOptions != nil {
		opts := o.MySQLOptions
		buf.WriteString(fmt.Sprintf("  Host: %s\n", opts.Host))
		buf.WriteString(fmt.Sprintf("  Username: %s\n", opts.Username))
		buf.WriteString("  Password: ********\n") // 安全考虑，不显示密码
		buf.WriteString(fmt.Sprintf("  Database: %s\n", opts.Database))
		buf.WriteString(fmt.Sprintf("  MaxIdleConnections: %d\n", opts.MaxIdleConnections))
		buf.WriteString(fmt.Sprintf("  MaxOpenConnections: %d\n", opts.MaxOpenConnections))
		buf.WriteString(fmt.Sprintf("  MaxConnectionLifeTime: %v\n", opts.MaxConnectionLifeTime))
		buf.WriteString(fmt.Sprintf("  LogLevel: %d\n", opts.LogLevel))
	} else {
		buf.WriteString("  <nil>\n")
	}
	buf.WriteString("\n")

	// JwtOptions
	buf.WriteString("JWT AUTHENTICATION OPTIONS:\n")
	buf.WriteString("---------------------------\n")
	if o.JwtOptions != nil {
		opts := o.JwtOptions
		buf.WriteString(fmt.Sprintf("  Realm: %s\n", opts.Realm))
		buf.WriteString(fmt.Sprintf("  Key: %s\n", opts.Key)) // 注意：实际生产中可能应该隐藏或部分隐藏
		buf.WriteString(fmt.Sprintf("  Timeout: %v\n", opts.Timeout))
		buf.WriteString(fmt.Sprintf("  MaxRefresh: %v\n", opts.MaxRefresh))
	} else {
		buf.WriteString("  <nil>\n")
	}
	buf.WriteString("\n")

	// Log Options
	buf.WriteString("LOGGING OPTIONS:\n")
	buf.WriteString("----------------\n")
	if o.Log != nil {
		opts := o.Log
		buf.WriteString(fmt.Sprintf("  OutputPaths: %v\n", opts.OutputPaths))
		buf.WriteString(fmt.Sprintf("  ErrorOutputPaths: %v\n", opts.ErrorOutputPaths))
		buf.WriteString(fmt.Sprintf("  Level: %s\n", opts.Level))
		buf.WriteString(fmt.Sprintf("  Format: %s\n", opts.Format))
		buf.WriteString(fmt.Sprintf("  DisableCaller: %t\n", opts.DisableCaller))
		buf.WriteString(fmt.Sprintf("  DisableStacktrace: %t\n", opts.DisableStacktrace))
		buf.WriteString(fmt.Sprintf("  EnableColor: %t\n", opts.EnableColor))
		buf.WriteString(fmt.Sprintf("  Development: %t\n", opts.Development))
		buf.WriteString(fmt.Sprintf("  Name: %s\n", opts.Name))
	} else {
		buf.WriteString("  <nil>\n")
	}
	buf.WriteString("\n")

	// InsecureServing
	buf.WriteString("INSECURE SERVING OPTIONS:\n")
	buf.WriteString("-------------------------\n")
	if o.InsecureServing != nil {
		opts := o.InsecureServing
		buf.WriteString(fmt.Sprintf("  BindAddress: %s\n", opts.BindAddress))
		buf.WriteString(fmt.Sprintf("  BindPort: %d\n", opts.BindPort))
	} else {
		buf.WriteString("  <nil>\n")
	}

	return buf.String()
}
