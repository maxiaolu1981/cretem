package options

import (
	"sync"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server"
)

var (
	serverDefaults     *server.Config
	serverDefaultsOnce sync.Once
)

func getServerDefaults() *server.Config {
	serverDefaultsOnce.Do(func() {
		serverDefaults = server.NewConfig()
	})
	return serverDefaults
}
