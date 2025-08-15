package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
)

func Run(cfg *config.Config) error {
	server, err := createAPIServer(cfg)
	if err != nil {
		return err
	}

	// 强制重新初始化日志
	//log.Init(cfg.Log)
	//log.Info("重新初始化后手动触发的测试日志")
	//log.Flush()

	return server.PrepareRun().Run()
}
