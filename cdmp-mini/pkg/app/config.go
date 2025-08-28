/*
这个包是一个专业的 Go 语言应用程序配置管理模块，基于 Viper 库实现了完整的配置加载和管理功能。以下是详细分析：

核心功能
1. 多源配置加载
支持从多个来源加载配置：

命令行参数：通过 -c/--config 指定配置文件

配置文件：支持 JSON、TOML、YAML、HCL、Java properties 等多种格式

环境变量：自动绑定并支持前缀和键名转换

2. 智能配置文件发现
采用分层搜索策略查找配置文件：

当前工作目录 (.)

用户主目录隐藏文件夹 (~/.appname/)

系统配置目录 (/etc/appname/)

自动识别文件格式（无需指定扩展名）

3. 环境变量集成
go
viper.AutomaticEnv() // 自动绑定环境变量
viper.SetEnvPrefix("APP_PREFIX") // 设置环境变量前缀
viper.SetEnvKeyReplacer() // 键名转换（. → _，- → _）
配置解析流程
初始化阶段 (init())
注册命令行标志：

go
pflag.StringVarP(&cfgFile, "config", "c", "", "配置文件路径")
配置加载阶段 (cobra.OnInitialize)
确定配置文件路径：命令行指定或自动发现

设置搜索路径：多级目录搜索

读取配置：viper.ReadInConfig()

错误处理：读取失败时优雅退出

配置展示阶段 (printConfig)
使用表格形式清晰展示所有配置项：

go
table := uitable.New()
table.AddRow("配置项:", "值")
关键技术特性
环境变量映射
自动将配置键名转换为环境变量：

server.port → SERVER_PORT

db-host → DB_HOST

智能路径处理
go
// 从应用名提取目录名
// "api-server" → "api"
names := strings.Split(basename, "-")
dirName := names[0]
多格式支持
Viper 自动检测并解析多种配置文件格式，无需手动指定。

使用示例
基本用法
bash
# 指定配置文件
./app -c config.yaml

# 自动发现配置（搜索 ./app.yaml, ~/.app/app, /etc/app/app 等）
./app
环境变量覆盖
bash
# 通过环境变量覆盖配置
export APP_SERVER_PORT=8080
export APP_LOG_LEVEL=debug
./app
设计优势
灵活性：支持多配置源，优先级明确（命令行 > 环境变量 > 配置文件）

易用性：自动发现配置，减少用户配置负担

可读性：表格形式清晰展示配置，便于调试

标准化：遵循 Twelve-Factor App 的配置最佳实践

扩展性：易于集成到各种命令行应用中

错误处理
配置文件读取失败时提供明确错误信息

环境变量绑定自动处理

多级配置搜索避免单点失败

生产环境特性
配置安全
支持隐藏目录配置（~/.appname/）

系统级配置目录支持

调试支持
完整配置项输出

清晰的错误信息提示

跨平台兼容
正确处理主目录路径（Windows/Linux/macOS）

统一的环境变量处理

这个配置管理包提供了一个企业级的配置解决方案，特别适合需要复杂配置管理的微服务和命令行工具，具有良好的可维护性和用户体验。

*/

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosuri/uitable"                                            // 用于生成格式化表格
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/homedir" // 获取用户主目录

	"github.com/spf13/cobra" // 命令行参数解析
	"github.com/spf13/pflag" // 命令行标志管理
	"github.com/spf13/viper" // 配置管理库
)

// 配置文件标志名（用于命令行参数）
const configFlagName = "config"

// 配置文件路径变量，通过命令行参数设置
var cfgFile string

// 初始化函数：注册命令行参数
// nolint: gochecknoinits 忽略"禁止在init中执行复杂逻辑"的检查
func init() {
	// 注册 -config/-c 标志：指定配置文件路径，支持多种格式
	pflag.StringVarP(&cfgFile, "config", "c", cfgFile, "从指定的文件读取配置，支持 JSON、TOML、YAML、HCL 或 Java properties 格式。")
}

// addConfigFlag 为特定服务添加配置标志，并初始化配置加载逻辑
// 参数：
//   - basename：服务名称（用于生成默认配置路径）
//   - fs：命令行标志集
func addConfigFlag(basename string, fs *pflag.FlagSet) {
	// 将已注册的 -config 标志添加到指定标志集
	fs.AddFlag(pflag.Lookup(configFlagName))

	// 启用环境变量自动绑定（viper 会自动读取环境变量）
	viper.AutomaticEnv()
	// 设置环境变量前缀（例如：服务名 "api-server" 对应前缀 "API_SERVER_"）
	viper.SetEnvPrefix(strings.Replace(strings.ToUpper(basename), "-", "_", -1))
	// 替换环境变量中的分隔符（例如：配置项 "log.level" 对应环境变量 "LOG_LEVEL"）
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// 在 cobra 初始化阶段执行配置加载
	cobra.OnInitialize(func() {
		if cfgFile != "" {
			// 若指定了配置文件路径，直接使用
			viper.SetConfigFile(cfgFile)
		} else {
			// 未指定路径时，添加默认搜索路径
			viper.AddConfigPath(".") // 当前目录

			// 若服务名包含 "-"（如 "core-service"），取第一部分作为目录名（如 "core"）
			if names := strings.Split(basename, "-"); len(names) > 1 {
				// 用户主目录下的隐藏目录（如 ~/.core/）
				viper.AddConfigPath(filepath.Join(homedir.HomeDir(), "."+names[0]))
				// 系统级配置目录（如 /etc/core/）
				viper.AddConfigPath(filepath.Join("/etc", names[0]))
			}

			// 设置默认配置文件名（无后缀，viper 会自动匹配格式）
			viper.SetConfigName(basename)
		}

		// 读取配置文件
		if err := viper.ReadInConfig(); err != nil {
			// 读取失败时输出错误并退出
			_, _ = fmt.Fprintf(os.Stderr, "错误：读取配置文件失败(%s)：%v\n", cfgFile, err)
			os.Exit(1)
		}
	})
}

// printConfig 打印所有加载的配置项（表格形式）
func printConfig() {
	// 获取所有配置项的键
	if keys := viper.AllKeys(); len(keys) > 0 {
		fmt.Printf("%v 配置项：\n", progressMessage) // progressMessage 应为外部定义的进度信息（如 "初始化中"）
		table := uitable.New()                   // 创建表格
		table.Separator = " "                    // 列分隔符为空格
		table.MaxColWidth = 80                   // 最大列宽 80（避免内容过长）
		table.RightAlign(0)                      // 第一列右对齐

		// 遍历所有配置项，添加到表格
		for _, k := range keys {
			table.AddRow(fmt.Sprintf("%s:", k), viper.Get(k)) // 格式："配置项:" + 值
		}

		fmt.Printf("%v", table) // 打印表格
	}
	fmt.Println()
}

/*
// 已注释的 loadConfig 函数：另一种配置加载实现（保留作为参考）
// 功能：读取配置文件和环境变量（若设置）
func loadConfig(cfg string, defaultName string) {
	if cfg != "" {
		viper.SetConfigFile(cfg) // 使用指定的配置文件
	} else {
		// 未指定时，添加默认搜索路径
		viper.AddConfigPath(".")
		viper.AddConfigPath(filepath.Join(homedir.HomeDir(), RecommendedHomeDir)) // 推荐的用户主目录下路径
		viper.SetConfigName(defaultName) // 默认配置文件名
	}

	viper.SetConfigType("yaml")              // 强制指定配置类型为 yaml
	viper.AutomaticEnv()                     // 自动读取环境变量
	viper.SetEnvPrefix(RecommendedEnvPrefix) // 环境变量前缀（如 IAM）
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_")) // 替换分隔符

	// 读取配置文件（若找到）
	if err := viper.ReadInConfig(); err != nil {
		log.Warnf("警告：viper 未能发现并加载配置文件：%s", err.Error())
	}
}
*/
