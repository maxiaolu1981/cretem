package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/app"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

const commandDesc = `IAM API 服务器用于验证和配置 API 对象的数据，这些对象包括用户、策略、密钥等。该 API 服务器通过处理 REST 操作来实现对这些 API 对象的管理。`

/*
1. 流程解释
这段代码是 API 服务器的入口逻辑，主要负责创建应用实例并定义启动流程，核心流程分为以下几步：
1.创建配置选项实例
通过 options.NewOptions() 初始化一个配置选项对象（opts），该对象包含服务器运行所需的所有可配置参数（如数据库连接、日志设置等）。

2.构建应用实例
调用 app.NewApp() 创建应用主体（application），并通过选项模式（app.WithXXX）配置应用的基本信息：
--应用名称和基准名称（basename）
--关联配置选项（opts）
--设置应用描述（commandDesc）
--配置默认的参数验证规则
--绑定核心运行函数（run(opts)）

3.定义运行逻辑
run 函数返回一个 app.RunFunc 类型的回调函数，作为应用的核心启动逻辑：
初始化日志系统（log.Init），并确保程序退出时刷新日志（defer log.Flush）
将配置选项（opts）转换为服务器运行所需的最终配置（cfg）
调用 Run(cfg) 启动 API 服务器核心服务
整个流程遵循 "配置→初始化→启动" 的逻辑，通过分层设计（配置选项、应用框架、核心服务）实现了解耦，便于维护和扩展。
*/

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp(basename, "iam apiserver",
		app.WithOptions(opts),
		app.WithDesriptions(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)
	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		log.Init(opts.Log)
		defer log.Flush()
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}
		return Run(cfg)

	}
}
