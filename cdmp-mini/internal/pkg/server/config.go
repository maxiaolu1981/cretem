/*
这是一个 Go 语言的服务器配置包，基于 Gin Web 框架，提供了完整的服务器配置和管理功能。
核心结构
1. InsecureServingInfo
处理非安全HTTP服务的配置：
# BindAddress - 绑定地址
# BindPort - 绑定端口
2. jwtInfo
JWT 认证配置：
# Realm - JWT 领域名称
# Key - JWT 密钥
# Timeout - JWT 超时时间
# MaxRefresh - 最大刷新时间
3. Config
主配置结构，包含：
服务器模式（Mode）
性能分析开关（EnableProfiling）
指标收集开关（EnableMetrics）
中间件列表（Middlewares）
健康检查开关（Healthz）
JWT 配置（Jwt）
服务配置（InsecureServingInfo）
4. CompleteConfig
完成配置的包装器，用于最终服务器创建
主要方法
NewConfig()
创建默认配置：
模式：发布模式（gin.ReleaseMode）
健康检查：启用
性能分析和指标收集：启用
中间件：空列表
JWT：默认配置（1秒超时和刷新）

Complete()
将基础配置转换为完成配置，为创建服务器做准备

New()
基于完成配置创建 GenericAPIServer 实例：
设置 Gin 运行模式
初始化 GenericAPIServer 结构
调用 initGenericAPIServer 进行最终初始化
设计特点
分层配置：基础配置 → 完成配置 → 服务器实例

默认安全：默认使用发布模式，避免开发配置泄露到生产环境
功能模块化：清晰分离不同功能模块的配置（JWT、中间件、监控等）
灵活扩展：通过中间件列表支持功能动态添加
*/
package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	Middlewares         []string
	Mode                string
	EnableMetrics       bool
	EnableProfiling     bool
	Healthz             bool
	Jwt                 *jwtInfo
	InsecureServingInfo *insecureServingInfo
}

func NewConfig() *Config {
	return &Config{
		Middlewares:     []string{},
		Mode:            gin.ReleaseMode,
		EnableMetrics:   true,
		EnableProfiling: true,
		Healthz:         true,
		Jwt: &jwtInfo{
			Realm:      "iam-apiserver",
			Timeout:    24 * time.Hour,
			MaxRefresh: 7 * 24 * time.Hour,
		},
		InsecureServingInfo: &insecureServingInfo{
			BindAdress: "127.0.0.1",
			BindPort:   8080,
		},
	}
}

type insecureServingInfo struct {
	BindAdress string
	BindPort   int
}

type jwtInfo struct {
	Realm      string
	Key        string
	Timeout    time.Duration
	MaxRefresh time.Duration
}
