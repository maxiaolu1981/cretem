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
该包定义了 JWT（JSON Web Token）认证的配置选项（JwtOptions），包含令牌签名密钥、有效期、刷新策略等核心参数，提供配置初始化、与服务器核心配置的映射、命令行参数绑定及合法性校验功能，是 API 服务器身份认证的核心配置模块。
函数流程详解
JwtOptions 结构体
存储 JWT 认证的核心配置参数，用于生成和验证 JWT 令牌：
Realm：令牌所属领域（展示给用户的标识，如 “iam jwt”）。
Key：用于签名 JWT 令牌的私钥（必须 6-32 字符，确保安全性）。
Timeout：令牌有效期（如 1 小时，过期后令牌失效）。
MaxRefresh：令牌最大可刷新时间（如 1 小时，过期后不允许刷新）。
NewJwtOptions 函数
功能：创建带有默认值的 JwtOptions 实例。
流程：通过 server.NewConfig() 获取服务器默认配置，将默认 JWT 配置（Realm、Key、Timeout 等）赋值给新实例，确保初始配置与服务器默认值一致。
ApplyTo 函数
功能：将当前 JWT 配置映射到服务器核心配置（server.Config）。
流程：构造 server.JwtInfo 结构体，填充 Realm、Key、Timeout、MaxRefresh 字段，赋值给 server.Config 的 Jwt 字段，使服务器运行时使用当前 JWT 配置。
Validate 函数
功能：校验 JWT 配置的合法性，重点检查签名密钥长度。
流程：使用 govalidator.StringLength 验证 Key 的长度是否在 6-32 字符之间（过短易被破解，过长可能影响性能），若不符合则添加错误信息。
AddFlags 函数
功能：为命令行解析绑定 JWT 相关参数，允许通过命令行覆盖默认配置。
流程：
绑定 --jwt.realm：指定令牌领域名称。
绑定 --jwt.key：指定签名私钥（核心参数，影响令牌安全性）。
绑定 --jwt.timeout：指定令牌有效期（支持时间单位，如 1h、30m）。
绑定 --jwt.max-refresh：指定令牌最大可刷新时间（控制令牌续期范围）。
*/

// options 包定义了 JWT 认证的配置选项，包含令牌签名、有效期、刷新策略等参数，
// 提供配置初始化、服务器核心配置映射、命令行参数绑定及合法性校验功能。
package options

import (
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/spf13/pflag"
)

// JwtOptions 包含与 API 服务器 JWT 认证功能相关的配置项
type JwtOptions struct {
	Realm      string        `json:"realm"       mapstructure:"realm"`       // 展示给用户的领域名称
	Key        string        `json:"key"         mapstructure:"key"`         // 用于签名 JWT 令牌的私钥
	Timeout    time.Duration `json:"timeout"     mapstructure:"timeout"`     // JWT 令牌有效期
	MaxRefresh time.Duration `json:"max-refresh" mapstructure:"max-refresh"` // 令牌最大可刷新时间（过期后不允许刷新）
}

// NewJwtOptions 创建带有默认参数的 JwtOptions 实例
func NewJwtOptions() *JwtOptions {
	defaults := server.NewConfig() // 获取服务器默认配置

	return &JwtOptions{
		Realm:      defaults.Jwt.Realm,      // 继承默认领域名称
		Key:        defaults.Jwt.Key,        // 继承默认签名密钥
		Timeout:    defaults.Jwt.Timeout,    // 继承默认令牌有效期
		MaxRefresh: defaults.Jwt.MaxRefresh, // 继承默认最大刷新时间
	}
}

// ApplyTo 将当前 JWT 配置应用到服务器核心配置（server.Config）
func (s *JwtOptions) ApplyTo(c *server.Config) error {
	// 构造服务器核心配置中的 JWT 信息，并赋值
	c.Jwt = &server.JwtInfo{
		Realm:      s.Realm,
		Key:        s.Key,
		Timeout:    s.Timeout,
		MaxRefresh: s.MaxRefresh,
	}

	return nil
}

// Validate 解析并校验程序启动时用户通过命令行输入的参数（主要校验签名密钥）
func (s *JwtOptions) Validate() []error {
	var errs []error

	// 校验签名密钥长度是否在 6-32 字符之间（确保安全性和性能平衡）
	if !govalidator.StringLength(s.Key, "6", "32") {
		errs = append(errs, fmt.Errorf("--secret-key 长度必须大于 5 且小于 33（即 6-32 字符）"))
	}

	return errs
}

// AddFlags 为特定 API 服务器添加与 JWT 功能相关的命令行参数到指定的 FlagSet
func (s *JwtOptions) AddFlags(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}

	// 绑定 --jwt.realm 参数：指定令牌领域名称
	fs.StringVar(&s.Realm, "jwt.realm", s.Realm, "展示给用户的领域名称。")

	// 绑定 --jwt.key 参数：指定 JWT 签名私钥（核心安全参数）
	fs.StringVar(&s.Key, "jwt.key", s.Key, "用于签名 JWT 令牌的私钥。")

	// 绑定 --jwt.timeout 参数：指定 JWT 令牌有效期
	fs.DurationVar(&s.Timeout, "jwt.timeout", s.Timeout, "JWT 令牌的有效期（如 1h 表示 1 小时）。")

	// 绑定 --jwt.max-refresh 参数：指定令牌最大可刷新时间
	fs.DurationVar(&s.MaxRefresh, "jwt.max-refresh", s.MaxRefresh, ""+
		"客户端可刷新令牌的最大时间范围，超过此时间后不允许刷新。")
}
