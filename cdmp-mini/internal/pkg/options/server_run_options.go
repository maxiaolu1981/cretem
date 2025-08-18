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

func(o *ServerRunOptions) AddFlags()(fs *pflag.FlagSet){
	 fs.StringVarP(&o.Mode,"server.mode",o.Mode,"以指定的服务器模式启动服务器。支持的服务器模式：调试（debug）、测试（test）、发布（release）。
")
    fs.BoolVarP(&o.Healthz,"server.healthz",o.Healthz,"添加自身就绪性检查并安装 /healthz 路由")
	fs.StringSliceVarP(&o.Middlewares,"server.middlewares",o.Middlewares,"服务器允许使用的中间件列表，以逗号分隔。若该列表为空，则将使用默认中间件")
    
}

func NewServerRunOptions() *ServerRunOptions {
	defaults := server.NewConfig()
	return &ServerRunOptions{
		Mode:        defaults.Mode,
		Healthz:     defaults.Healthz,
		Middlewares: defaults.Middlewares,
	}
}

func 