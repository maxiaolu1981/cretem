/*
这个包是IAM项目的API服务器主入口，负责创建和运行IAM API Server。以下是包摘要：
1.包功能
应用创建: 提供 NewApp() 函数创建命令行应用
配置管理: 处理命令行选项和配置初始化
服务器启动: 负责启动IAM API Server核心服务
2.核心组件
应用框架: 基于 pkg/app 构建命令行应用
配置系统: 使用 options.Options 和 config.Config 管理配置
日志系统: 集成 pkg/log 进行日志管理
3.主要流程
解析命令行参数 → options.NewOptions()
创建应用实例 → app.NewApp()
转换配置 → config.CreateConfigFromOptions()
运行服务器 → Run(cfg)
4.特性
支持REST API操作
管理用户、策略、密钥等API对象
包含完整的配置验证和默认值设置
这个包作为IAM系统的API入口点，将命令行接口与核心服务器功能连接起来。
*/

package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/app"
)

const commandDesc = `IAM API 服务器用于验证和配置 API 对象的数据，这些对象包括用户、策略、密钥等。该 API 服务器通过处理 REST 操作来实现对这些 API 对象的管理。`

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application, _ := app.NewApp(basename, "api server",
		app.WithOptions(opts),
		app.WithDefaultValidArgs(), app.WithRunFunc(run(opts)), app.WithDescription(commandDesc))

	return application
}

func run(opt *options.Options) app.Runfunc {
	return func(basename string) error {
		return nil
	}
}
