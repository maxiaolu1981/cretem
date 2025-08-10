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
该包定义了通用 API 服务器的核心配置结构与初始化逻辑，负责管理服务器的运行参数（如安全 / 非安全服务配置、JWT 认证信息、运行模式等）、加载外部配置（从文件或环境变量），并基于配置创建可运行的 API 服务器实例（基于 Gin 框架），是服务器启动的核心模块。
函数流程详解
核心结构体定义
Config：存储服务器的完整配置，包括安全服务（HTTPS）、非安全服务（HTTP）、JWT 认证、运行模式、中间件列表、健康检查开关等核心参数。
CertKey：管理 TLS 证书文件路径（证书文件与私钥文件），用于安全服务配置。
SecureServingInfo：HTTPS 服务配置，包含绑定地址、端口及证书信息，提供 Address() 方法生成完整服务地址（如 0.0.0.0:8443）。
InsecureServingInfo：HTTP 服务配置，仅包含服务地址。
JwtInfo：JWT 认证配置，包括领域名称、签名密钥、令牌有效期及最大刷新时间。
CompletedConfig：已完成的配置（包装 Config），用于最终创建服务器实例。
配置初始化与处理函数
NewConfig()：创建默认配置实例，设置默认值（如运行模式为 gin.ReleaseMode、健康检查启用、JWT 有效期 1 小时等）。
Config.Complete()：将基础配置转换为 “已完成配置”（CompletedConfig），为服务器实例化做准备（通常用于补全或校验配置）。
CompletedConfig.New()：基于已完成的配置创建 GenericAPIServer 实例：
先设置 Gin 框架的运行模式（如 release/debug）。
初始化服务器实例，包含安全 / 非安全服务配置、健康检查开关、中间件列表等，并关联 Gin 引擎（gin.New()）。
调用 initGenericAPIServer（未完全展示，推测用于初始化路由、中间件等核心逻辑）。
配置加载函数
LoadConfig(cfg, defaultName)：从配置文件或环境变量加载配置：
若指定了配置文件路径（cfg），直接使用该文件；否则从默认路径（当前目录、用户家目录 .iam 文件夹、/etc/iam）加载名为 defaultName 的配置文件。
支持 YAML 格式，自动读取环境变量（前缀为 IAM，如 IAM_SERVER_MODE 对应 server.mode 配置）。
若加载失败，仅警告不终止（允许使用默认配置）
*/
// server 包定义了通用 API 服务器的配置结构与初始化逻辑，负责管理服务器参数、加载外部配置并创建可运行的服务器实例。
package server

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/util/homedir"
	"github.com/maxiaolu1981/cretem/nexuscore/log"

	"github.com/spf13/viper"
)

const (
	// RecommendedHomeDir 用于存放所有 IAM 服务配置的默认目录
	RecommendedHomeDir = ".iam"

	// RecommendedEnvPrefix 所有 IAM 服务环境变量的前缀
	RecommendedEnvPrefix = "IAM"
)

// Config 用于配置 GenericAPIServer 的结构体，字段按对开发者的重要性排序
type Config struct {
	SecureServing   *SecureServingInfo   // 安全服务（HTTPS）配置
	InsecureServing *InsecureServingInfo // 非安全服务（HTTP）配置
	Jwt             *JwtInfo             // JWT 认证配置
	Mode            string               // 服务器运行模式（对应 Gin 的模式：debug/test/release）
	Middlewares     []string             // 启用的中间件列表
	Healthz         bool                 // 是否启用健康检查（/healthz 路由）
	EnableProfiling bool                 // 是否启用性能分析（pprof）
	EnableMetrics   bool                 // 是否启用指标收集（/metrics 路由）
}

// CertKey 包含与 TLS 证书相关的配置项
type CertKey struct {
	// CertFile 包含 PEM 编码的证书文件（可能包含完整证书链）
	CertFile string
	// KeyFile 包含与 CertFile 对应的 PEM 编码私钥文件
	KeyFile string
}

// SecureServingInfo 存放 TLS 服务器（HTTPS）的配置
type SecureServingInfo struct {
	BindAddress string  // 绑定的 IP 地址（如 0.0.0.0）
	BindPort    int     // 绑定的端口（如 8443）
	CertKey     CertKey // TLS 证书信息
}

// Address 将主机 IP 和端口拼接为完整地址（如 0.0.0.0:8443）
func (s *SecureServingInfo) Address() string {
	return net.JoinHostPort(s.BindAddress, strconv.Itoa(s.BindPort))
}

// InsecureServingInfo 存放非安全 HTTP 服务器的配置
type InsecureServingInfo struct {
	Address string // 服务地址（如 127.0.0.1:8080）
}

// JwtInfo 定义用于创建 JWT 认证中间件的字段
type JwtInfo struct {
	// Realm 显示给用户的领域名称，默认为 "iam jwt"
	Realm string
	// Key 用于签名 JWT 令牌的密钥，默认为空（需在初始化时设置）
	Key string
	// Timeout 令牌有效期，默认为 1 小时
	Timeout time.Duration
	// MaxRefresh 令牌最大可刷新时间，默认为 0（不允许刷新）
	MaxRefresh time.Duration
}

// NewConfig 返回一个带有默认值的 Config 结构体
func NewConfig() *Config {
	return &Config{
		Healthz:         true,            // 默认启用健康检查
		Mode:            gin.ReleaseMode, // 默认使用 Gin 的 release 模式
		Middlewares:     []string{},      // 默认启用空中间件列表（后续可通过配置补充）
		EnableProfiling: true,            // 默认启用性能分析
		EnableMetrics:   true,            // 默认启用指标收集
		Jwt: &JwtInfo{
			Realm:      "iam jwt",     // 默认领域名称
			Timeout:    1 * time.Hour, // 默认令牌有效期 1 小时
			MaxRefresh: 1 * time.Hour, // 默认最大刷新时间 1 小时
		},
	}
}

// CompletedConfig 是 GenericAPIServer 的完整配置（已完成校验或补全）
type CompletedConfig struct {
	*Config
}

// Complete 补全配置中未设置但必需的字段（基于已有字段推导），若需应用选项应先调用 ApplyOptions
// 注意：此方法会修改调用者本身
func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{c}
}

// New 从已完成的配置创建 GenericAPIServer 实例
func (c CompletedConfig) New() (*GenericAPIServer, error) {
	// 在创建 Gin 引擎前设置运行模式
	gin.SetMode(c.Mode)

	s := &GenericAPIServer{
		SecureServingInfo:   c.SecureServing,   // 安全服务配置
		InsecureServingInfo: c.InsecureServing, // 非安全服务配置
		healthz:             c.Healthz,         // 健康检查开关
		enableMetrics:       c.EnableMetrics,   // 指标收集开关
		enableProfiling:     c.EnableProfiling, // 性能分析开关
		middlewares:         c.Middlewares,     // 启用的中间件列表
		Engine:              gin.New(),         // 初始化 Gin 引擎
	}

	// 初始化 GenericAPIServer（推测用于注册路由、中间件等核心逻辑）
	initGenericAPIServer(s)

	return s, nil
}

// LoadConfig 读取配置文件和环境变量（若已设置）
func LoadConfig(cfg string, defaultName string) {
	if cfg != "" {
		// 若指定了配置文件路径，直接使用该文件
		viper.SetConfigFile(cfg)
	} else {
		// 否则从默认路径加载配置
		viper.AddConfigPath(".")                                                  // 当前目录
		viper.AddConfigPath(filepath.Join(homedir.HomeDir(), RecommendedHomeDir)) // 用户家目录的 .iam 文件夹
		viper.AddConfigPath("/etc/iam")                                           // 系统级配置目录
		viper.SetConfigName(defaultName)                                          // 配置文件名称（不含后缀）
	}

	// 配置文件类型为 yaml
	viper.SetConfigType("yaml")
	// 自动读取环境变量中与配置匹配的项
	viper.AutomaticEnv()
	// 环境变量前缀为 IAM（如 IAM_SERVER_MODE 对应 server.mode）
	viper.SetEnvPrefix(RecommendedEnvPrefix)
	// 替换环境变量中的分隔符（如将配置中的 . 或 - 转为 _，适应环境变量命名习惯）
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// 读取配置文件（若存在）
	if err := viper.ReadInConfig(); err != nil {
		log.Warnf("警告：viper 未能发现并加载配置文件：%s", err.Error())
	}
}
