/*
这个 Go 代码包 options 提供了不安全 HTTP 服务端口的配置选项。以下是主要功能的摘要：

核心结构体
go
type InsecureServingOptions struct {
    BindAddress string `json:"bind-address" mapstructure:"bind-address"`
    BindPort    int    `json:"bind-port"    mapstructure:"bind-port"`
}
BindAddress: 绑定 IP 地址（默认：127.0.0.1）

BindPort: 绑定端口号（默认：8080）

主要方法
1. 构造函数
go
func NewInsecureServingOptions() *InsecureServingOptions
创建带有默认值的配置选项实例。

2. 参数验证
go
func (s *InsecureServingOptions) Validate() []error
验证端口号是否在有效范围内（0-65535），0 表示禁用不安全端口。

3. 命令行标志
go
func (s *InsecureServingOptions) AddFlags(fs *pflag.FlagSet)
添加命令行参数：

--insecure.bind-address: 绑定 IP 地址

--insecure.bind-port: 绑定端口号

4. 配置应用
go
func (s *InsecureServingOptions) ApplyTo(c *server.Config) error
将选项配置应用到服务器的配置结构中。

设计特点
安全性提醒: 明确标注这是不安全的服务端口，不建议使用

灵活性: 支持通过命令行参数覆盖默认配置

验证机制: 确保端口号在合法范围内

配置映射: 支持 JSON 和 mapstructure 标签，便于配置解析

使用场景
适用于需要提供不加密、无认证的 HTTP 服务的内部应用或开发环境，生产环境建议使用安全的 HTTPS 服务。
原始配置 → Complete()（补全默认值） → Validate()（验证合法性） → ApplyTo(target)（应用到目标） → 目标生效。

*/

package options

import (
	"net"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/spf13/pflag"
)

type InsecureServingOptions struct {
	BindAddress string `json:"bind-address" mapstructure:"bind-address"`
	BindPort    int    `json:"bind-port"    mapstructure:"bind-port"`
}

func NewInsecureServingOptions() *InsecureServingOptions {
	return &InsecureServingOptions{
		BindAddress: "127.0.0.1",
		BindPort:    8080,
	}
}

func (i *InsecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&i.BindAddress, "insecure.bind-address", "b", i.BindAddress, "用于监听 --insecure.bind-port（不安全绑定端口）的 IP 地址（若需监听所有 IPv4 接口，设为 0.0.0.0；若需监听所有 IPv6 接口，设为 ::）")
	fs.IntVarP(&i.BindPort, "insecure.bind-port", "p", i.BindPort, "用于提供未加密、未认证访问的端口。默认假设已配置防火墙规则，确保该端口无法从部署机器外部访问，且 IAM 公网地址的 443 端口会被代理到该端口（默认配置下由 nginx 实现此代理）。设为 0 可禁用该端口。")
}

func (i *InsecureServingOptions) Validate() []error {
	var errs = []error{}
	if i.BindAddress == "" {
		errs = append(errs, errors.WithCode(code.ErrValidation, "绑定的地址不能为空"))
	}
	if net.ParseIP(i.BindAddress) == nil {
		errs = append(errs, errors.WithCode(code.ErrValidation, "无效的ip地址%s", i.BindAddress))
	}
	if i.BindPort < 0 || i.BindPort > 65535 {
		errs = append(errs, errors.WithCode(code.ErrValidation, "端口必须在0-65535之间"))
	}
	if i.BindPort != 0 {
		address := net.JoinHostPort(i.BindAddress, strconv.Itoa(i.BindPort))
		if _, err := net.ResolveTCPAddr("tcp", address); err != nil {
			errs = append(errs, errors.WithCode(code.ErrValidation, "地址+端口组合无效%v", err))
		}
	}
	return errs
}
