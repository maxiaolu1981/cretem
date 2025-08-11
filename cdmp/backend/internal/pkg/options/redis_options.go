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
该包定义了 Redis 服务的配置选项（RedisOptions），包含 Redis 单机 / 集群的连接参数（如地址、端口、认证信息）、连接池优化配置及安全相关设置，提供配置初始化、命令行参数绑定及（预留）合法性校验功能，用于管理 Redis 服务的连接逻辑。
函数流程详解
RedisOptions 结构体
存储 Redis 服务的核心配置参数，支持单机、集群两种模式，主要包含：
连接基础信息：Host（主机名）、Port（端口）、Addrs（集群地址列表）、Username（用户名）、Password（密码）、Timeout（连接超时时间）。
数据库与模式：Database（数据库编号，集群模式下需为 0）、EnableCluster（是否启用集群模式）、MasterName（主实例名称，用于主从架构）。
连接池优化：MaxIdle（最大空闲连接数）、MaxActive（最大活动连接数）。
安全设置：UseSSL（是否启用 SSL 加密）、SSLInsecureSkipVerify（是否跳过 SSL 证书验证，用于自签名证书）。
NewRedisOptions 函数
功能：创建带有默认值的 RedisOptions 实例。
流程：初始化结构体并设置默认参数，包括：
单机默认地址 127.0.0.1:6379，数据库编号 0。
连接池默认最大空闲连接 2000、最大活动连接 4000（适配高可用场景）。
默认禁用集群模式、SSL 加密及证书跳过验证。
Validate 函数
功能：校验 Redis 配置的合法性（当前为预留接口，未实现具体逻辑）。
潜在扩展：未来可添加校验，如集群模式下 Database 必须为 0、Addrs 不能为空，或 Timeout 不能为负数等。
AddFlags 函数
功能：为命令行解析绑定 Redis 相关参数，允许通过命令行覆盖默认配置。
流程：按参数类别绑定命令行标志，包括：
基础连接参数：--redis.host、--redis.port、--redis.addrs 等。
数据库与模式参数：--redis.database（说明集群模式下需为 0）、--redis.enable-cluster 等。
连接池优化参数：--redis.optimisation-max-idle、--redis.optimisation-max-active（说明生产环境建议值）。
安全参数：--redis.use-ssl、--redis.ssl-insecure-skip-verify（说明自签名证书场景）。

*/
// options 包定义了 Redis 服务的配置选项，包含连接参数、集群模式、连接池优化及安全设置，
// 提供配置初始化、命令行参数绑定及合法性校验（预留）功能，用于管理 Redis 连接逻辑。
package options

import (
	"github.com/spf13/pflag"
)

// RedisOptions 定义 Redis 集群/单机的配置选项
type RedisOptions struct {
	Host                  string   `json:"host"                     mapstructure:"host"`                     // Redis 服务主机地址
	Port                  int      `json:"port"                     mapstructure:"port"`                     // Redis 服务端口
	Addrs                 []string `json:"addrs"                    mapstructure:"addrs"`                    // Redis 地址集合（格式：127.0.0.1:6379，集群模式使用）
	Username              string   `json:"username"                 mapstructure:"username"`                 // 访问 Redis 服务的用户名
	Password              string   `json:"password"                 mapstructure:"password"`                 // Redis 数据库的认证密码（可选）
	Database              int      `json:"database"                 mapstructure:"database"`                 // 数据库编号（默认 0，集群模式下必须为 0）
	MasterName            string   `json:"master-name"              mapstructure:"master-name"`              // Redis 主实例的名称（主从架构使用）
	MaxIdle               int      `json:"optimisation-max-idle"    mapstructure:"optimisation-max-idle"`    // 连接池最大空闲连接数
	MaxActive             int      `json:"optimisation-max-active"  mapstructure:"optimisation-max-active"`  // 连接池最大活动连接数
	Timeout               int      `json:"timeout"                  mapstructure:"timeout"`                  // 连接 Redis 服务的超时时间（秒）
	EnableCluster         bool     `json:"enable-cluster"           mapstructure:"enable-cluster"`           // 是否启用 Redis 集群模式（开启插槽模式）
	UseSSL                bool     `json:"use-ssl"                  mapstructure:"use-ssl"`                  // 是否启用 SSL 加密连接（用于支持传输加密的 Redis 服务）
	SSLInsecureSkipVerify bool     `json:"ssl-insecure-skip-verify" mapstructure:"ssl-insecure-skip-verify"` // 连接加密 Redis 时，是否允许自签名证书
}

// NewRedisOptions 创建一个默认值的 RedisOptions 实例
func NewRedisOptions() *RedisOptions {
	return &RedisOptions{
		Host:                  "127.0.0.1", // 默认主机地址
		Port:                  6379,        // 默认端口
		Addrs:                 []string{},  // 默认空地址列表（集群模式需手动配置）
		Username:              "",          // 默认无用户名
		Password:              "",          // 默认无密码
		Database:              0,           // 默认数据库编号 0
		MasterName:            "",          // 默认无主实例名称
		MaxIdle:               2000,        // 默认最大空闲连接数 2000
		MaxActive:             4000,        // 默认最大活动连接数 4000
		Timeout:               0,           // 默认无超时（使用底层库默认值）
		EnableCluster:         false,       // 默认禁用集群模式
		UseSSL:                false,       // 默认禁用 SSL 加密
		SSLInsecureSkipVerify: false,       // 默认不跳过 SSL 证书验证
	}
}

// Validate 验证 Redis 配置参数的合法性（当前未实现具体逻辑）
func (o *RedisOptions) Validate() []error {
	errs := []error{} // 可扩展：添加集群模式下 database 必须为 0 等校验

	return errs
}

// AddFlags 为特定 API 服务器添加与 Redis 存储相关的命令行参数到指定的 FlagSet
func (o *RedisOptions) AddFlags(fs *pflag.FlagSet) {
	// 绑定 --redis.host 参数：Redis 服务器主机名
	fs.StringVar(&o.Host, "redis.host", o.Host, "Redis 服务器的主机名。")

	// 绑定 --redis.port 参数：Redis 服务器监听端口
	fs.IntVar(&o.Port, "redis.port", o.Port, "Redis 服务器监听的端口。")

	// 绑定 --redis.addrs 参数：Redis 地址集合（集群模式使用）
	fs.StringSliceVar(&o.Addrs, "redis.addrs", o.Addrs, "Redis 地址集合（格式：127.0.0.1:6379）。")

	// 绑定 --redis.username 参数：Redis 访问用户名
	fs.StringVar(&o.Username, "redis.username", o.Username, "访问 Redis 服务的用户名。")

	// 绑定 --redis.password 参数：Redis 认证密码
	fs.StringVar(&o.Password, "redis.password", o.Password, "Redis 数据库的可选认证密码。")

	// 绑定 --redis.database 参数：数据库编号（集群模式限制）
	fs.IntVar(&o.Database, "redis.database", o.Database, ""+
		"默认数据库为 0。Redis 集群不支持设置数据库，因此若 --redis.enable-cluster=true，该值应省略或显式设为 0。")

	// 绑定 --redis.master-name 参数：主实例名称
	fs.StringVar(&o.MasterName, "redis.master-name", o.MasterName, "Redis 主实例的名称。")

	// 绑定 --redis.optimisation-max-idle 参数：最大空闲连接数
	fs.IntVar(&o.MaxIdle, "redis.optimisation-max-idle", o.MaxIdle, ""+
		"此设置用于配置空闲（无流量）时连接池保持的连接数。将 --redis.optimisation-max-active 设为较大值，"+
		"高可用部署通常设为 2000 左右。")

	// 绑定 --redis.optimisation-max-active 参数：最大活动连接数
	fs.IntVar(&o.MaxActive, "redis.optimisation-max-active", o.MaxActive, ""+
		"为避免过度提交 Redis 服务器连接，可限制 Redis 的最大活动连接数。生产环境建议设为约 4000。")

	// 绑定 --redis.timeout 参数：连接超时时间
	fs.IntVar(&o.Timeout, "redis.timeout", o.Timeout, "连接 Redis 服务的超时时间（秒）。")

	// 绑定 --redis.enable-cluster 参数：是否启用集群模式
	fs.BoolVar(&o.EnableCluster, "redis.enable-cluster", o.EnableCluster, ""+
		"若使用 Redis 集群，启用此选项以开启插槽模式。")

	// 绑定 --redis.use-ssl 参数：是否启用 SSL 加密
	fs.BoolVar(&o.UseSSL, "redis.use-ssl", o.UseSSL, ""+
		"若设置，IAM 将假设与 Redis 的连接已加密（用于支持传输中加密的 Redis 提供商）。")

	// 绑定 --redis.ssl-insecure-skip-verify 参数：是否跳过 SSL 证书验证
	fs.BoolVar(&o.SSLInsecureSkipVerify, "redis.ssl-insecure-skip-verify", o.SSLInsecureSkipVerify, ""+
		"连接加密的 Redis 数据库时，允许使用自签名证书。")
}
