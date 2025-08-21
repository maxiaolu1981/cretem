/*
	apiserver.NewApp("iam-apiserver")  // 初始化应用框架（配置+命令）
  ↓
App.Run()                          // 触发 Cobra 命令解析
  ↓
cobra.Command.Execute()            // 调用 runCommand
  ↓
runCommand → run()                 // 初始化日志、转换配置
  ↓
Run(cfg)                           // 创建 apiServer 实例
  ↓
createAPIServer()                  // 初始化 HTTP 服务和业务组件
  ↓
PrepareRun()                       // 注册路由和中间件
  ↓
Run()                              // 启动服务，监听请求
*/
/*

				一. apiServer.NewApp("iam-apiserver")
						作用:创建应用实例,搭建服务的基础框架,准备服务器基础配置和运行逻辑
						核心流程:
						1.创建配置容器
					     --通过options.NewOptions()初始化配置对象(*options.Options)
					     -- InsecureServingInfo:非安全服务器端口:127.0.0.1:8080
						 -- SeccurServingInfo:安全服务器端口:127.0.0.1:8443
						 -- mysql数据库配置
						 -- 日志级别参数(通用配置+业务配置)
						2.绑定应用元数据: 服务名称 描述信息  命令行标志
						3.注册核心运行函数:通过app.WithRunc(run(opts))
						4.初始化Cobra命令:创建Cobra根命令(*Cobra.Command),关联配置和运行函数
			    二. App.Run():触发启动流程
			       App.Run()是启动流程的开关,触发cobr框架解析命令行参数并执行核心逻辑
				   核心操作:
				   1.执行Cobra命令入口:cobra.Command.Execute(),负责触发绑定RunE函数(a.runcommand())
				   2.执行核心启动逻辑(App.runcommand()),调用之前注册的run函数
				   3.run函数核心操作:
				     -- 初始化日志系统log.Init(opts.Log)
					 -- 配置转换与教研:调用config.CreateConfigFromOpetions(opts)将原始配置从options.Options转换为服务器所需的最终配置(*config.Config),包括默认值填充和格式校验
					 --调用Run(cfg)进入服务器实例创建和启动阶段
	            三. Run(cfg):创建服务器实例
				作用:基于最终配置(*config.Config)创建并且启动apiServer实例(服务聚合器)
				核心操作:
				1.创建服务器实例:createAPIServer(cfg)生成*apiServer实例:
				内部包括:
				http服务(genericAPIServer):gin,处理http请求
				其他服务:gRPCAPIServer
				2.准备启动:server.PrepareRun():完成路由器注册和中间件初始化
				// 3.运行server.RUn()启动http和gRPC服务,监听配置端口:8080,进入阻塞状态处理请求
			四.服务运行与关闭
			1.运行阶段:genericAPIServer持续监听http请求,通过注册的路由和中间件处理请求(如认证 业务逻辑执行 响应返回)
			2.关闭阶段:受到中断信号,如ctrl+c ,apiServer中的优雅关闭管理器(gs)协调所有服务停止:先停止接受新请，，等待正在处理的请求完成,最后释资源.
*/
package main

import (
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	apiserver.NewApp("iam-apiserver").Run()
}
