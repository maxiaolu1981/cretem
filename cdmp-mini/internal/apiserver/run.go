package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
)

//这段行代码定义了 API 服务器的核心启动流程，主要负责创建服务器实例并启动服务，具体流程如下：
//Run
//作用：
// 1. 调用 createAPIServer 函数，根据最终配置 cfg（由 config.CreateConfigFromOptions 生成）创建一个 API 服务器实例（通常是 *APIServer 或类似结构体）。
//内部逻辑：createAPIServer 会基于配置初始化服务器的核心组件，例如：
//初始化 HTTP 路由（注册接口、中间件）
//关联数据库连接、缓存等依赖
//配置服务器参数（端口、超时时间等）
//错误处理：如果创建过程失败（如配置错误、依赖初始化失败），//直接返回错误，终止启动。
//（2）准备并启动服务器
/*
server.PrepareRun()：服务器启动前的准备工作，通常包括：
验证服务器状态（确保所有组件已初始化）
启动辅助服务（如健康检查、监控指标收集）
注册优雅关闭钩子（确保程序退出时资源正确释放）
返回一个可执行的运行器（通常是包含 Run 方法的结构体）。
Run()：启动服务器的核心方法，实际执行：
监听指定端口（如 cfg.Port）
启动 HTTP 服务（开始处理客户端请求）
阻塞当前 goroutine，保持服务运行（直到收到中断信号，如 Ctrl+C）
*/

func Run(cfg *config.Config) error {
	apiServer, err := createAPIServer(cfg)
	if err != nil {
		return err
	}
	return apiServer.PrepareRun().Run()
}
