/*
该包定义了非安全 HTTP 服务的配置选项（InsecureServingOptions），包含服务绑定的 IP 地址、端口等参数，提供配置初始化、与服务器核心配置的映射、命令行参数绑定及合法性校验功能，用于管理未加密、未认证的 HTTP 服务运行设置（注意：该配置不建议在生产环境使用）
函数流程详解
1.InsecureServingOptions 结构体
存储非安全 HTTP 服务的核心配置参数：
BindAddress：服务绑定的 IP 地址（默认 127.0.0.1，即仅本地可访问）。
BindPort：服务监听的端口（默认 8080，设置为 0 可禁用该服务）。
字段通过 json 和 mapstructure 标签支持从配置文件解析。
2.NewInsecureServingOptions 函数
功能：创建带有默认值的 InsecureServingOptions 实例。
流程：初始化结构体并设置默认参数（绑定地址 127.0.0.1、端口 8080），确保服务启动时有基础的非安全访问配置（默认仅本地可访问，降低安全风险）。
3.ApplyTo 函数
功能：将当前非安全服务配置映射到服务器核心配置（server.Config）。
流程：通过 net.JoinHostPort 拼接 BindAddress 和 BindPort 为完整地址（如 127.0.0.1:8080），并赋值给 server.Config 的 InsecureServing 字段，完成配置的传递。
Validate 函数
功能：校验非安全服务配置的合法性，主要检查端口范围。
流程：若 BindPort 小于 0 或大于 65535（超出有效端口范围），则添加错误信息到返回列表，避免因无效端口导致服务启动失败。
AddFlags 函数
功能：为命令行解析绑定非安全服务相关参数，允许通过命令行覆盖默认配置。
流程：
绑定 --insecure.bind-address 参数，指定服务的绑定 IP 地址，说明支持的接口（如 0.0.0.0 或 ::）。
绑定 --insecure.bind-port 参数，指定服务的监听端口，说明其安全约束（默认仅本地可访问，通过 nginx 代理到 443 端口）及禁用方式（设置为 0）。
*/
// options 包定义了非安全 HTTP 服务的配置选项，包含绑定地址、端口等参数，
// 提供配置初始化、与服务器核心配置的映射、命令行参数绑定及合法性校验功能（不建议生产环境使用）。
package options

import (
	"fmt"
	"net"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/spf13/pflag"
)

// InsecureServingOptions 用于配置非安全 HTTP 服务（未认证、未授权的不安全端口，不建议生产环境使用）
type InsecureServingOptions struct {
	BindAddress string `json:"bind-address" mapstructure:"bind-address"` // 服务绑定的 IP 地址
	BindPort    int    `json:"bind-port"    mapstructure:"bind-port"`    // 服务监听的端口
}

// NewInsecureServingOptions 创建带有默认值的 InsecureServingOptions 实例（返回非安全配置，不建议生产使用）
func NewInsecureServingOptions() *InsecureServingOptions {
	return &InsecureServingOptions{
		BindAddress: "127.0.0.1", // 默认绑定本地回环地址（仅本地可访问）
		BindPort:    8080,        // 默认监听端口 8080
	}
}

// ApplyTo 将当前非安全服务配置应用到服务器核心配置（server.Config）
func (s *InsecureServingOptions) ApplyTo(c *server.Config) error {
	// 拼接 IP 地址和端口为完整服务地址（如 127.0.0.1:8080），并设置到核心配置
	c.InsecureServing = &server.InsecureServingInfo{
		Address: net.JoinHostPort(s.BindAddress, strconv.Itoa(s.BindPort)),
	}

	return nil
}

// Validate 解析并校验程序启动时用户通过命令行输入的参数
func (s *InsecureServingOptions) Validate() []error {
	var errors []error

	// 校验端口是否在有效范围（0-65535），0 表示禁用非安全端口
	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--insecure.bind-port %v 必须在 0-65535 范围内（含 0 和 65535），0 表示关闭非安全（HTTP）端口",
				s.BindPort,
			),
		)
	}

	return errors
}

// AddFlags 为特定 API 服务器添加与非安全服务相关的命令行参数到指定的 FlagSet
func (s *InsecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	// 绑定 --insecure.bind-address 参数，指定非安全服务的绑定地址
	fs.StringVar(&s.BindAddress, "insecure.bind-address", s.BindAddress, ""+
		"用于监听 --insecure.bind-port 的 IP 地址（设置为 0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口）。")

	// 绑定 --insecure.bind-port 参数，指定非安全服务的监听端口
	fs.IntVar(&s.BindPort, "insecure.bind-port", s.BindPort, ""+
		"用于提供未加密、未认证访问的端口。默认情况下，防火墙规则会确保该端口只能从部署机器内部访问，"+
		"且 IAM 公共地址的 443 端口会代理到该端口（默认配置中由 nginx 实现）。设置为 0 可禁用。")
}
