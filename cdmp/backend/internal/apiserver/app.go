// 该包实现了 IAM API 服务器的核心启动逻辑，负责初始化应用配置、解析命令行参数、设置日志系统，并最终启动 //API 服务以处理用户、策略、密钥等 API 对象的 REST 操作。
/*
设计思路:
在这段代码中，config.CreateConfigFromOptions(opts) 将 opts 转换为 cfg 而非直接使用 opts，主要基于以下设计考虑：
抽象与封装
options.Options 通常只负责原始配置数据（如命令行参数、配置文件内容的直接映射）
config.Config 则是经过加工处理的运行时配置，可能包含验证、默认值填充、格式转换等逻辑
这种分层使配置处理逻辑与原始数据存储分离，符合单一职责原则
扩展灵活性
当前实现中 Config 只是简单嵌入 Options，但预留了扩展空间：
go
// 未来可能的扩展
type Config struct {
    *options.Options
    DBConfig     *database.Config  // 派生配置
    CacheConfig  *cache.Config     // 转换后配置
    Validated    bool              // 验证标记
}
转换过程中可以加入配置校验、数据清洗等逻辑，确保 cfg 是可用且安全的配置
依赖注入优化
后续代码（如 Run(cfg)）依赖的是 config.Config 接口而非具体的 options.Options
这种抽象允许未来替换配置来源（如从服务发现获取配置），而无需修改使用配置的代码
配置隔离
opts 可能包含一些临时配置（如命令行帮助选项、版本信息开关）
转换为 cfg 可以过滤掉这些运行时不需要的配置项，提供更纯净的配置视图
简单来说，这是一种 "数据转换 - 使用分离" 的设计模式，虽然当前实现较简单，但为后续的配置管理扩展提供了清晰的架构基础。
*/
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

func NewApp(basename string) *app.App {
	// 初始化命令行选项（包含默认配置和可解析的参数定义）
	opts := options.NewOptions()

	application := app.NewApp(
		"IAM API Server",
		basename,
		app.WithOptions(opts),
		//app.WithNoConfig(),
		//app.WithNoVersion(),              //选项
		app.WithDescription(commandDesc), // 绑定
		app.WithDefaultValidArgs(),       // 使用规则,不允许有参数注入
		app.WithRunFunc(run(opts)),       //
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
