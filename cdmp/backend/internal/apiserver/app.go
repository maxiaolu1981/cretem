// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
// package apiserver
// app.go
// 该包实现了 IAM API 服务器的核心启动逻辑，负责初始化应用配置、解析命令行参数、设置日志系统，并最终启动 //API 服务以处理用户、策略、密钥等 API 对象的 REST 操作。
package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/app"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

const commandDesc = `IAM API 服务器负责验证和配置 API 对象的数据，这些对象包括用户、策略、密钥等。API 服务器通过处理 REST 操作来管理这些 API 对象。
如需了解更多关于 iam-apiserver 的信息，请访问：
https://github.com/maxiaolu1981/cretem/blob/master/cdmp/doc/docs/guide/cmd/iam-apiserver.md`

// NewApp 创建一个带有默认参数的应用实例
// 参数 basename 为程序名称（可执行文件名）
func NewApp(basename string) *app.App {
	// 初始化命令行选项（包含默认配置和可解析的参数定义）
	opts := options.NewOptions()

	// 创建应用实例，配置基本信息和回调函数
	application := app.NewApp(
		"IAM API Server",                 // 应用名称
		basename,                         // 程序名
		app.WithOptions(opts),            // 绑定命令行选项
		app.WithDescription(commandDesc), // 绑定功能描述
		app.WithDefaultValidArgs(),       // 使用默认的参数验证规则
		app.WithRunFunc(run(opts)),       // 绑定应用启动后的运行函数
	)

	return application
}

// run 定义应用启动后的核心逻辑，返回一个符合 app.RunFunc 接口的函数
// 参数 opts 为解析后的命令行选项
func run(opts *options.Options) app.RunFunc {
	// 返回的匿名函数将在应用初始化完成后执行
	return func(basename string) error {
		// 初始化日志系统，根据 opts 中的日志配置（如级别、输出路径等）
		log.Init(opts.Log)
		// 确保程序退出时刷新日志缓冲区，避免日志丢失
		defer log.Flush()

		// 根据命令行选项生成最终的服务配置（合并默认值、环境变量、命令行参数等）
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err // 配置生成失败时返回错误
		}

		// 启动 API 服务器核心服务（具体实现由 Run 函数提供）
		return Run(cfg)
	}
}
