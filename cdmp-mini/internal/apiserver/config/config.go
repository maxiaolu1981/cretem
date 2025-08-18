package config

import "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"

type config struct {
	*options.Options
}

func NewConfig(opt *options.Options) *config {
	return &config{opt}
}
