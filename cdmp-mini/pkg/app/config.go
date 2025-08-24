/*
该包（package app）主要负责应用程序的配置加载与管理，基于 viper 库实现配置文件的读取、环境变量的解析，并提供配置信息的打印功能，方便调试和配置校验。
核心流程
初始化配置标志位：定义 -config（短选项 -c）命令行参数，用于指定配置文件路径。
绑定配置与环境变量：将配置项与环境变量关联，支持通过环境变量覆盖配置文件值。
加载配置文件：根据命令行参数或默认路径（当前目录、用户主目录、系统目录）查找并读取配置文件（支持 JSON、TOML、YAML 等格式）。
打印配置信息：将加载的所有配置项以表格形式输出，便于查看当前生效的配置

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
