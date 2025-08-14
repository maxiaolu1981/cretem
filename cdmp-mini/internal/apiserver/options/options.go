package options

import (
	genericoptions "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
)

// 定义Options参数结构
type Options struct {
	MySQLOptions *genericoptions.MySQLOptions `json:"mysql"`
	Log          *log.Options
}

func NewOptions() *Options {
	return &Options{
		MySQLOptions: genericoptions.NewMySQLOptions(),
		Log:          log.NewOptions(),
	}
}
