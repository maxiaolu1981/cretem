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
/*
该包定义了安全 HTTPS 服务的配置选项（SecureServingOptions）及相关结构体，用于管理 HTTPS 服务的绑定地址、端口、TLS 证书等核心参数。包含配置初始化、与服务器核心配置的映射、命令行参数绑定、合法性校验及证书路径补全功能，是安全 HTTP 服务（带认证和授权）的核心配置模块。
核心结构体说明
SecureServingOptions
安全 HTTPS 服务的主配置，包含服务绑定信息和 TLS 证书配置：
BindAddress：HTTPS 服务绑定的 IP 地址（默认 0.0.0.0，即所有 IPv4 接口）。
BindPort：HTTPS 服务监听的端口（默认 8443，Required 为 true 时不可为 0）。
Required：标记端口是否为必需（true 时端口必须在 1-65535 之间，不可为 0）。
ServerCert：TLS 证书相关配置（类型为 GeneratableKeyCert）。
CertKey
存储 TLS 证书和私钥文件的路径：
CertFile：PEM 编码的证书文件路径（可包含完整证书链）。
KeyFile：与证书匹配的 PEM 编码私钥文件路径。
GeneratableKeyCert
处理 TLS 证书的生成或显式指定，支持两种模式（显式文件 / 目录生成）：
CertKey：显式指定的证书和私钥文件（优先于目录生成）。
CertDirectory：证书生成目录（若未显式指定证书文件，将在此目录生成）。
PairName：证书文件名前缀（与 CertDirectory 配合生成路径：<dir>/<name>.crt 和 <dir>/<name>.key）。
函数流程详解
1. NewSecureServingOptions()
功能：创建带有默认值的 SecureServingOptions 实例。
流程：
默认绑定地址为 0.0.0.0（所有 IPv4 接口），端口 8443。
标记为必需服务（Required: true），即端口不可为 0。
初始化证书配置：默认证书目录 /var/run/iam，配对名称 iam（生成证书文件为 /var/run/iam/iam.crt 和 /var/run/iam/iam.key）。
2. ApplyTo(c *server.Config) error
功能：将当前安全服务配置映射到服务器核心配置（server.Config）。
流程：
构造 server.SecureServingInfo 结构体，填充绑定地址、端口及证书文件路径（从 ServerCert.CertKey 提取）。
将该结构体赋值给 server.Config 的 SecureServing 字段，完成配置传递。
3. Validate() []error
功能：校验安全服务配置的合法性（主要检查端口范围）。
流程：
若 Required 为 true：端口必须在 1-65535 之间（不可为 0），否则添加错误。
若 Required 为 false：端口可在 0-65535 之间（0 表示禁用），超出范围则添加错误。
返回所有校验错误（空切片表示无错误）。
4. AddFlags(fs *pflag.FlagSet)
功能：为命令行解析绑定安全服务相关参数，允许通过命令行覆盖默认配置。
流程：
绑定 --secure.bind-address：指定 HTTPS 服务的绑定 IP 地址。
绑定 --secure.bind-port：指定 HTTPS 服务的端口（说明是否可禁用，取决于 Required）。
绑定证书相关参数：
--secure.tls.cert-dir：证书目录（若未显式指定证书文件则使用）。
--secure.tls.pair-name：证书文件名前缀（与目录配合生成路径）。
--secure.tls.cert-key.cert-file：显式指定的证书文件路径。
--secure.tls.cert-key.private-key-file：显式指定的私钥文件路径。
5. Complete() error
功能：补全证书文件路径（当未显式指定证书文件时，基于目录和配对名生成）。
流程：
若已显式指定 CertFile 或 KeyFile，直接返回（不处理）。
若指定了 CertDirectory 但未指定 PairName，返回错误（缺少文件名前缀）。
若同时指定 CertDirectory 和 PairName，自动生成证书路径：CertFile = <dir>/<name>.crt，KeyFile = <dir>/<name>.key。
6. CreateListener(addr string) (net.Listener, int, error)
功能：根据指定地址创建网络监听器（TCP），并返回监听器和实际监听端口。
流程：
调用 net.Listen("tcp", addr) 监听指定地址（如 0.0.0.0:8443）。
从监听器地址中提取 TCP 端口（转换为 *net.TCPAddr 后获取）。
若监听失败或地址无效，返回错误；否则返回监听器和端口。

*/
// options 包定义了安全 HTTPS 服务的配置选项，包含绑定地址、端口、TLS 证书等参数，
// 提供配置初始化、服务器核心配置映射、命令行参数绑定、合法性校验及证书路径补全功能。
package options

import (
	"fmt"
	"net"
	"path"

	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/spf13/pflag"
)

// SecureServingOptions 包含 HTTPS 服务启动相关的配置项（带认证和授权）
type SecureServingOptions struct {
	BindAddress string             `json:"bind-address" mapstructure:"bind-address"` // HTTPS 服务绑定的 IP 地址
	BindPort    int                `json:"bind-port"    mapstructure:"bind-port"`    // HTTPS 服务监听的端口（当设置 Listener 时忽略，即使为 0 也会启动 HTTPS）
	Required    bool               // 标记端口是否为必需（true 表示 BindPort 不能为 0）
	ServerCert  GeneratableKeyCert `json:"tls"          mapstructure:"tls"` // 用于安全通信的 TLS 证书信息
}

// CertKey 包含与 TLS 证书相关的配置项
type CertKey struct {
	CertFile string `json:"cert-file"        mapstructure:"cert-file"`        // 包含 PEM 编码证书的文件（可包含完整证书链）
	KeyFile  string `json:"private-key-file" mapstructure:"private-key-file"` // 包含与 CertFile 匹配的 PEM 编码私钥的文件
}

// GeneratableKeyCert 包含可生成的 TLS 证书相关配置（支持显式指定文件或自动生成）
type GeneratableKeyCert struct {
	CertKey       CertKey `json:"cert-key" mapstructure:"cert-key"`   // 显式指定的证书和私钥文件（优先使用）
	CertDirectory string  `json:"cert-dir"  mapstructure:"cert-dir"`  // 生成证书的目录（若未显式指定证书文件，将在此目录生成）
	PairName      string  `json:"pair-name" mapstructure:"pair-name"` // 证书文件名前缀（与 CertDirectory 配合生成路径：<dir>/<name>.crt 和 <dir>/<name>.key）
}

// NewSecureServingOptions 创建带有默认参数的 SecureServingOptions 实例
func NewSecureServingOptions() *SecureServingOptions {
	return &SecureServingOptions{
		BindAddress: "0.0.0.0", // 默认绑定所有 IPv4 接口
		BindPort:    8443,      // 默认监听端口 8443
		Required:    true,      // 默认要求必须启用 HTTPS 服务（端口不可为 0）
		ServerCert: GeneratableKeyCert{
			PairName:      "iam",          // 默认证书文件名前缀
			CertDirectory: "/var/run/iam", // 默认证书生成目录
		},
	}
}

// ApplyTo 将当前安全服务配置应用到服务器核心配置（server.Config）
func (s *SecureServingOptions) ApplyTo(c *server.Config) error {
	// 安全服务必须提供 HTTPS 支持，构造核心配置中的 SecureServingInfo
	c.SecureServing = &server.SecureServingInfo{
		BindAddress: s.BindAddress,
		BindPort:    s.BindPort,
		CertKey: server.CertKey{
			CertFile: s.ServerCert.CertKey.CertFile, // 证书文件路径
			KeyFile:  s.ServerCert.CertKey.KeyFile,  // 私钥文件路径
		},
	}

	return nil
}

// Validate 解析并校验程序启动时用户通过命令行输入的参数（主要校验端口）
func (s *SecureServingOptions) Validate() []error {
	if s == nil {
		return nil
	}

	errors := []error{}

	// 校验端口范围：Required 为 true 时，端口必须在 1-65535 之间（不可为 0）
	if s.Required && (s.BindPort < 1 || s.BindPort > 65535) {
		errors = append(
			errors,
			fmt.Errorf(
				"--secure.bind-port %v 必须在 1-65535 范围内（含 1 和 65535），且不能设为 0 禁用",
				s.BindPort,
			),
		)
	} else if s.BindPort < 0 || s.BindPort > 65535 {
		// Required 为 false 时，端口可在 0-65535 之间（0 表示禁用）
		errors = append(
			errors,
			fmt.Errorf(
				"--secure.bind-port %v 必须在 0-65535 范围内（含 0 和 65535），0 表示关闭安全端口",
				s.BindPort,
			),
		)
	}

	return errors
}

// AddFlags 为特定 API 服务器添加与 HTTPS 服务相关的命令行参数到指定的 FlagSet
func (s *SecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	// 绑定 --secure.bind-address 参数：HTTPS 服务的绑定 IP 地址
	fs.StringVar(&s.BindAddress, "secure.bind-address", s.BindAddress, ""+
		"用于监听 --secure.bind-port 端口的 IP 地址。相关接口必须可被引擎其他部分及 CLI/web 客户端访问。"+
		"若为空，将使用所有接口（0.0.0.0 表示所有 IPv4 接口，:: 表示所有 IPv6 接口）。")

	// 绑定 --secure.bind-port 参数：HTTPS 服务的端口（说明是否可禁用）
	desc := "用于提供带认证和授权的 HTTPS 服务的端口。"
	if s.Required {
		desc += " 该端口不能设为 0 禁用。"
	} else {
		desc += " 若设为 0，则不提供 HTTPS 服务。"
	}
	fs.IntVar(&s.BindPort, "secure.bind-port", s.BindPort, desc)

	// 绑定 --secure.tls.cert-dir 参数：TLS 证书目录（若未显式指定证书文件则使用）
	fs.StringVar(&s.ServerCert.CertDirectory, "secure.tls.cert-dir", s.ServerCert.CertDirectory, ""+
		"TLS 证书所在目录。若提供了 --secure.tls.cert-key.cert-file 和 --secure.tls.cert-key.private-key-file，"+
		"此参数将被忽略。")

	// 绑定 --secure.tls.pair-name 参数：证书文件名前缀（与目录配合生成路径）
	fs.StringVar(&s.ServerCert.PairName, "secure.tls.pair-name", s.ServerCert.PairName, ""+
		"与 --secure.tls.cert-dir 配合使用的证书和密钥文件名前缀，最终文件为 <cert-dir>/<pair-name>.crt 和 <cert-dir>/<pair-name>.key。")

	// 绑定 --secure.tls.cert-key.cert-file 参数：显式指定的证书文件
	fs.StringVar(&s.ServerCert.CertKey.CertFile, "secure.tls.cert-key.cert-file", s.ServerCert.CertKey.CertFile, ""+
		"用于 HTTPS 的默认 x509 证书文件（若有 CA 证书，需跟在服务器证书后）。")

	// 绑定 --secure.tls.cert-key.private-key-file 参数：显式指定的私钥文件
	fs.StringVar(&s.ServerCert.CertKey.KeyFile, "secure.tls.cert-key.private-key-file",
		s.ServerCert.CertKey.KeyFile, ""+
			"与 --secure.tls.cert-key.cert-file 匹配的默认 x509 私钥文件。")
}

// Complete 补全配置中未显式设置但必需的字段（主要补全证书文件路径）
func (s *SecureServingOptions) Complete() error {
	// 若安全服务未启用（端口为 0）或为空，直接返回
	if s == nil || s.BindPort == 0 {
		return nil
	}

	keyCert := &s.ServerCert.CertKey
	// 若已显式指定证书或私钥文件，无需补全
	if len(keyCert.CertFile) != 0 || len(keyCert.KeyFile) != 0 {
		return nil
	}

	// 若指定了证书目录但未指定文件名前缀，返回错误
	if len(s.ServerCert.CertDirectory) > 0 {
		if len(s.ServerCert.PairName) == 0 {
			return fmt.Errorf("若设置 --secure.tls.cert-dir，则必须同时设置 --secure.tls.pair-name")
		}
		// 基于目录和前缀生成证书和私钥路径
		keyCert.CertFile = path.Join(s.ServerCert.CertDirectory, s.ServerCert.PairName+".crt")
		keyCert.KeyFile = path.Join(s.ServerCert.CertDirectory, s.ServerCert.PairName+".key")
	}

	return nil
}

// CreateListener 根据指定地址创建网络监听器（TCP），并返回监听器和实际监听端口
func CreateListener(addr string) (net.Listener, int, error) {
	network := "tcp" // 使用 TCP 协议

	// 监听指定地址
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, 0, fmt.Errorf("监听地址 %v 失败：%w", addr, err)
	}

	// 提取实际监听的端口
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close() // 关闭监听器
		return nil, 0, fmt.Errorf("无效的监听地址：%q", ln.Addr().String())
	}

	return ln, tcpAddr.Port, nil
}
