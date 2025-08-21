/*
该包定义了 gRPC 服务的配置选项（GRPCOptions），包含 gRPC 服务的绑定地址、端口、最大消息大小等参数，提供配置初始化、命令行参数绑定及合法性校验功能，用于管理 gRPC 服务的基础运行设置。
函数流程详解
1.GRPCOptions 结构体
存储 gRPC 服务的核心配置参数：
BindAddress：gRPC 服务绑定的 IP 地址（默认 0.0.0.0，即监听所有 IPv4 接口）。
BindPort：gRPC 服务监听的端口（默认 8081，设置为 0 可禁用）。
MaxMsgSize：gRPC 消息的最大字节数（默认 4*1024*1024，即 4MB）。
2.NewGRPCOptions 函数
功能：创建带有默认值的 GRPCOptions 实例。
流程：初始化结构体并设置默认参数（绑定地址 0.0.0.0、端口 8081、最大消息大小 4MB），确保服务启动时有合理的基础配置。
Validate 函数
功能：校验 gRPC 配置的合法性，主要检查端口范围。
流程：若 BindPort 小于 0 或大于 65535（无效端口范围），则添加错误信息到返回列表，避免因无效端口导致服务启动失败。
注意：错误信息中误写为 --insecure-port（应为 --grpc.bind-port），实际使用时需修正。
AddFlags 函数
功能：为命令行解析绑定 gRPC 相关参数，允许通过命令行覆盖默认配置。
流程：
绑定 --grpc.bind-address 参数，指定 gRPC 服务的绑定地址，说明支持的接口（如 0.0.0.0 或 ::）。
绑定 --grpc.bind-port 参数，指定 gRPC 服务的监听端口，说明其安全约束（默认仅本地可访问，通过 nginx 代理到 443 端口）。
绑定 --grpc.max-msg-size 参数，设置 gRPC 消息的最大大小。
*/
// options 包定义了 gRPC 服务的配置选项，包含绑定地址、端口、最大消息大小等参数，
// 提供配置初始化、命令行参数绑定及合法性校验功能。
package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

// GRPCOptions 用于配置 gRPC 服务的参数（未认证、未授权的不安全端口配置，不建议在生产环境使用）
type GRPCOptions struct {
	BindAddress string `json:"bind-address" mapstructure:"bind-address"` // gRPC 服务绑定的 IP 地址
	BindPort    int    `json:"bind-port"    mapstructure:"bind-port"`    // gRPC 服务监听的端口
	MaxMsgSize  int    `json:"max-msg-size" mapstructure:"max-msg-size"` // gRPC 消息的最大字节数
}

// NewGRPCOptions 创建带有默认值的 GRPCOptions 实例（返回未认证、未授权的不安全配置，不建议生产使用）
func NewGRPCOptions() *GRPCOptions {
	return &GRPCOptions{
		BindAddress: "0.0.0.0",       // 默认绑定所有 IPv4 接口
		BindPort:    8081,            // 默认监听端口 8081
		MaxMsgSize:  4 * 1024 * 1024, // 默认最大消息大小为 4MB（4*1024*1024 字节）
	}
}

// Validate 解析并校验程序启动时用户通过命令行输入的参数
func (s *GRPCOptions) Validate() []error {
	var errors []error

	// 校验端口是否在有效范围（0-65535），0 表示禁用不安全端口
	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--grpc.bind-port %v 必须在 0-65535 范围内（含 0 和 65535），0 表示关闭 gRPC 端口",
				s.BindPort,
			),
		)
	}

	return errors
}

// AddFlags 为特定 API 服务器添加与 gRPC 功能相关的命令行参数到指定的 FlagSet
func (s *GRPCOptions) AddFlags(fs *pflag.FlagSet) {
	// 绑定 --grpc.bind-address 参数，指定 gRPC 服务的绑定地址
	fs.StringVar(&s.BindAddress, "grpc.bind-address", s.BindAddress, ""+
		"用于监听 --grpc.bind-port 的 IP 地址（设置为 0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口）。")

	// 绑定 --grpc.bind-port 参数，指定 gRPC 服务的监听端口
	fs.IntVar(&s.BindPort, "grpc.bind-port", s.BindPort, ""+
		"用于提供未加密、未认证的 gRPC 访问的端口。默认情况下，防火墙规则会确保该端口只能从部署机器内部访问，"+
		"且 IAM 公共地址的 443 端口会代理到该端口（默认配置中由 nginx 实现）。设置为 0 可禁用。")

	// 绑定 --grpc.max-msg-size 参数，设置 gRPC 消息的最大大小
	fs.IntVar(&s.MaxMsgSize, "grpc.max-msg-size", s.MaxMsgSize, "gRPC 最大消息大小（字节）。")
}
