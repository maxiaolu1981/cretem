package options

import "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"

type FeatureOptions struct {
	EnableProfiling bool `json:"profiling"      mapstructure:"profiling"`
	EnableMetrics   bool `json:"enable-metrics" mapstructure:"enable-metrics"`
}

func (o *FeatureOptions) ApplyTo(c *server.Config) error {
	c.EnableMetrics = o.EnableMetrics
	c.EnableProfiling = o.EnableProfiling
	return nil
}
