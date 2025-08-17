package options

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
	"github.com/spf13/pflag"
)

type ServerRunOptions struct {
	Mode        string   `json:"mode"        mapstructure:"mode"`
	Healthz     bool     `json:"healthz"     mapstructure:"healthz"`
	Middlewares []string `json:"middlewares" mapstructure:"middlewares"`
}

// ApplyTo将ServerRunOptions的值写回Config,完成最终配置
func (s *ServerRunOptions) ApplyTo(c *server.Config) error {
	c.Mode = s.Mode
	c.Healthz = s.Healthz
	c.Middlewares = s.Middlewares
	return nil
}

func NewServerRunOptions() *ServerRunOptions {
	defaults := server.NewConfig()
	return &ServerRunOptions{
		Mode:        defaults.Mode,
		Healthz:     defaults.Healthz,
		Middlewares: defaults.Middlewares,
	}
}

func (o *ServerRunOptions) AddFlags(fss *pflag.FlagSet) {
	fss.StringVar(&o.Mode, "server.mode", o.Mode, "Start the server in a specified server mode. Supported server mode: debug, test, release")
}
