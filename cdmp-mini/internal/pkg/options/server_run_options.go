package options

import (
	"fmt"
	"regexp"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
	"github.com/spf13/pflag"
)

type ServerRunOptions struct {
	Mode        string   `json:"mode"        mapstructure:"mode"`
	Healthz     bool     `json:"healthz"     mapstructure:"healthz"`
	Middlewares []string `json:"middlewares" mapstructure:"middlewares"`
}

func (o *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.StringVar(&o.Mode, "server.mode", o.Mode, "以指定的服务器模式启动服务器。支持的服务器模式：调试（debug）、测试（test）、发布（release.")
	fs.BoolVar(&o.Healthz, "server.healthz", o.Healthz, "添加自身就绪性检查并安装 /healthz 路由")
	fs.StringSliceVar(&o.Middlewares, "server.middlewares", o.Middlewares, "服务器允许使用的中间件列表，以逗号分隔。若该列表为空，则将使用默认中间件.")
}

func (s *ServerRunOptions) Validate() []error {
	errs := []error{}
	allowModes := map[string]struct{}{
		"debug":   {},
		"test":    {},
		"release": {},
	}
	if _, ok := allowModes[s.Mode]; ok {
		errs = append(errs, fmt.Errorf("不支持mode输入:%s,输入必须在`debug,test,release`之间."))
	}

	for i, mid := range s.Middlewares {
		if mid == "" {
			errs = append(errs, fmt.Errorf("%d输入不能为空.", i))
			continue
		}
		if !regexp.MustCompile(`^[0-9a-zA-Z_-]+$`).MatchString(mid) {
			errs = append(errs, fmt.Errorf("无效的中间件名称:[%s],索引[%d].必须是由数字,小写字母,大写字母或者_-组成", i, mid))
		}
	}
	return errs
}

func NewServerRunOptions() *ServerRunOptions {
	defaults := server.NewConfig()
	return &ServerRunOptions{
		Mode:        defaults.Mode,
		Healthz:     defaults.Healthz,
		Middlewares: defaults.Middlewares,
	}
}

// ApplyTo applies the run options to the method receiver and returns self.
func (s *ServerRunOptions) ApplyTo(c *server.Config) error {
	c.Mode = s.Mode
	c.Healthz = s.Healthz
	c.Middlewares = s.Middlewares

	return nil
}
