package apiserver

import "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"

func Run(cfg *config.Config) error {
	server, err := createAPIServer(cfg)
}
