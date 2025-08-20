package config

import "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"

type Config struct {
	*options.Options
}

func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
