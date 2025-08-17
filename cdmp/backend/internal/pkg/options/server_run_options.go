/*
该包定义了通用 API 服务器的运行配置选项（ServerRunOptions），包含服务器运行模式、健康检查开关、中间件列表等核心参数，提供配置初始化、参数验证、命令行参数绑定等功能，是服务器运行配置的基础模块。
函数流程详解
ServerRunOptions 结构体
存储服务器运行的核心配置参数：
Mode：服务器运行模式（如 debug、test、release）
Healthz：是否启用健康检查（/healthz 路由）
Middlewares：允许使用的中间件列表（逗号分隔）
字段通过 json 和 mapstructure 标签支持从配置文件解析。
NewServerRunOptions 函数
功能：创建带有默认值的 ServerRunOptions 实例。
流程：先通过 server.NewConfig() 获取服务器默认配置，再将默认配置中的模式、健康检查状态、中间件列表赋值给新实例，确保初始配置合理。
ApplyTo 函数
功能：将当前配置应用到服务器核心配置（server.Config）。
流程：将 ServerRunOptions 的字段（Mode/Healthz/Middlewares）直接赋值给目标配置对象，完成配置映射。
Validate 函数
功能：验证配置的合法性。
现状：当前实现为空（返回空错误列表），实际使用时可补充校验逻辑（如检查 Mode 是否为支持的类型、Middlewares 是否包含无效名称等）。
AddFlags 函数
功能：为命令行解析绑定参数，允许通过命令行覆盖默认配置。
流程：
为 Mode 绑定 --server.mode 命令行参数，说明支持的运行模式。
为 Healthz 绑定 --server.healthz 参数，控制健康检查开关。
为 Middlewares 绑定 --server.middlewares 参数，指定允许的中间件列表。

*/

// options 包定义了通用 API 服务器的运行配置选项，包含服务器模式、健康检查、中间件等核心参数，
// 提供配置初始化、参数绑定和验证功能。
package options

import (
	"fmt"
	"strings"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/spf13/pflag"
)

// ServerRunOptions 包含通用 API 服务器运行时的配置选项
type ServerRunOptions struct {
	Mode        string   `json:"mode"        mapstructure:"mode"`        // 服务器运行模式（debug/test/release）
	Healthz     bool     `json:"healthz"     mapstructure:"healthz"`     // 是否启用健康检查（/healthz 路由）
	Middlewares []string `json:"middlewares" mapstructure:"middlewares"` // 允许使用的中间件列表（逗号分隔）
}

// 默认值传递→用户配置→最终应用
// NewServerRunOptions 创建带有默认参数的 ServerRunOptions 实例
func NewServerRunOptions() *ServerRunOptions {
	defaults := server.NewConfig() // 获取服务器默认配置

	return &ServerRunOptions{
		Mode:        defaults.Mode,        // 从默认配置继承运行模式
		Healthz:     defaults.Healthz,     // 从默认配置继承健康检查开关状态
		Middlewares: defaults.Middlewares, // 从默认配置继承中间件列表
	}
}

// ApplyTo 将当前配置应用到服务器核心配置（server.Config）
func (s *ServerRunOptions) ApplyTo(c *server.Config) error {
	c.Mode = s.Mode               // 覆盖服务器模式
	c.Healthz = s.Healthz         // 覆盖健康检查开关
	c.Middlewares = s.Middlewares // 覆盖中间件列表

	return nil
}

// Validate 验证配置的合法性（当前未实现具体校验逻辑）
func (s *ServerRunOptions) Validate() []error {
	var errores []error

	// 1. 验证服务器运行模式是否为支持的类型
	supportedModes := map[string]bool{
		"debug":   true,
		"test":    true,
		"release": true,
	}
	if !supportedModes[s.Mode] {
		errores = append(errores, errors.New(
			"无效的服务器运行模式，支持的模式为：debug、test、release",
		))
	}

	// 2. 验证中间件列表是否包含空字符串（避免配置错误）
	for i, mid := range s.Middlewares {
		if strings.TrimSpace(mid) == "" {
			errores = append(errores, errors.New(
				fmt.Sprintf("中间件列表中存在空值（索引：%d），请检查配置", i),
			))
		}
	}

	// 3. （可选）如果有不允许的中间件名称，可在此添加校验
	// 例如：禁止使用 "forbidden-middleware"
	// forbiddenMids := map[string]bool{"forbidden-middleware": true}
	// for _, mid := range s.Middlewares {
	// 	if forbiddenMids[mid] {
	// 		errors = append(errors, errors.New(
	// 			fmt.Sprintf("不允许使用的中间件：%s", mid),
	// 		))
	// 	}
	// }

	return errores
}

// AddFlags 为命令行解析绑定参数，允许通过命令行参数修改配置
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	// 绑定 --server.mode 参数，指定服务器运行模式
	fs.StringVar(&s.Mode, "server.mode", s.Mode, ""+
		"以指定模式启动服务器，支持的模式：debug、test、release。")

	// 绑定 --server.healthz 参数，控制是否启用健康检查
	fs.BoolVar(&s.Healthz, "server.healthz", s.Healthz, ""+
		"添加自身就绪检查并安装 /healthz 路由。")

	// 绑定 --server.middlewares 参数，指定允许的中间件列表
	fs.StringSliceVar(&s.Middlewares, "server.middlewares", s.Middlewares, ""+
		"服务器允许的中间件列表，用逗号分隔。若为空，将使用默认中间件。")
}
