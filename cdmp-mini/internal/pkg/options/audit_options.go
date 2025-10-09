package options

import (
	"time"

	"github.com/spf13/pflag"
)

// AuditOptions 控制审计功能的开关与落地方式。
type AuditOptions struct {
	Enabled         bool          `json:"enabled" mapstructure:"enabled"`
	BufferSize      int           `json:"bufferSize" mapstructure:"bufferSize"`
	ShutdownTimeout time.Duration `json:"shutdownTimeout" mapstructure:"shutdownTimeout"`
	LogFile         string        `json:"logFile" mapstructure:"logFile"`
	EnableMetrics   bool          `json:"enableMetrics" mapstructure:"enableMetrics"`
}

func NewAuditOptions() *AuditOptions {
	return &AuditOptions{
		Enabled:         true,
		BufferSize:      512,
		ShutdownTimeout: 5 * time.Second,
		LogFile:         "log/audit.log",
		EnableMetrics:   true,
	}
}

func (o *AuditOptions) Complete() {
	if o.BufferSize <= 0 {
		o.BufferSize = 512
	}
	if o.ShutdownTimeout <= 0 {
		o.ShutdownTimeout = 5 * time.Second
	}
	if o.LogFile == "" {
		o.LogFile = "log/audit.log"
	}
}

func (o *AuditOptions) Validate() []error {
	return nil
}

func (o *AuditOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Enabled, "audit.enabled", o.Enabled, "是否开启审计功能")
	fs.IntVar(&o.BufferSize, "audit.buffer-size", o.BufferSize, "审计事件缓冲队列大小")
	fs.DurationVar(&o.ShutdownTimeout, "audit.shutdown-timeout", o.ShutdownTimeout, "服务退出时等待审计队列耗尽的超时时间")
	fs.StringVar(&o.LogFile, "audit.log-file", o.LogFile, "审计文件落地路径(JSON Lines)")
	fs.BoolVar(&o.EnableMetrics, "audit.enable-metrics", o.EnableMetrics, "是否将审计事件写入指标监控")
}
