package options

import (
	"net"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
)

type InsecureServingOptions struct {
	BindAddress string `json:"bind-address" mapstructure:"bind-address"`
	BindPort    int    `json:"bind-port"    mapstructure:"bind-port"`
}

func (s *InsecureServingOptions) ApplyTo(c *server.Config) error {
	c.InsecureServing = &server.InsecureServingInfo{
		Address: net.JoinHostPort(s.BindAddress, strconv.Itoa(s.BindPort)),
	}
	return nil
}
