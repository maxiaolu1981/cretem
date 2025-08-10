package main

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
)

// 基础配置（可能未补全、未校验）
type Config struct {
	Mode string // 运行模式（必填）
	Port int    // 端口（必填，1-65535）
}

// 完成配置（标记配置已补全、已校验）
// 核心：仅该类型能创建服务器，防止未处理的 Config 被直接使用
type CompletedConfig struct {
	*Config
}

// 服务器实例
type APIServer struct {
	mode string
	port int
}

// --------------- 关键：仅 CompletedConfig 能创建服务器 ---------------
// NewServer 为 CompletedConfig 的方法，确保只有完成的配置能创建服务器
func (cc CompletedConfig) NewServer() *APIServer {
	return &APIServer{
		mode: cc.Mode,
		port: cc.Port,
	}
}

// --------------- Config 的补全与校验逻辑 ---------------
// Complete 补全并校验配置，返回 CompletedConfig（转换为“完成状态”）
func (c *Config) Complete() (CompletedConfig, error) {
	// 1. 补全默认值（处理未显式设置的字段）
	if c.Mode == "" {
		c.Mode = gin.ReleaseMode // 默认 release 模式
	}

	// 2. 校验配置合法性
	if c.Port <= 0 || c.Port > 65535 {
		return CompletedConfig{}, errors.New("端口必须在 1-65535 之间")
	}
	if c.Mode != gin.DebugMode && c.Mode != gin.TestMode && c.Mode != gin.ReleaseMode {
		return CompletedConfig{}, errors.New("模式必须是 debug/test/release")
	}

	// 3. 转换为 CompletedConfig（标记配置已就绪）
	return CompletedConfig{Config: c}, nil
}

func main() {
	// 场景1：错误用法：直接使用未完成的 Config 创建服务器（编译报错！）
	//invalidConfig := &Config{Port: 8080} // 缺少 Mode，且未调用 Complete()
	// 错误原因：NewServer 是 CompletedConfig 的方法，Config 类型无法调用
	// server := invalidConfig.NewServer() // 编译时直接报错：Config 没有 NewServer 方法

	// 场景2：正确用法：先补全为 CompletedConfig，再创建服务器
	validConfig := &Config{Port: 8080} // 初始缺少 Mode
	// 步骤1：补全并校验配置（获取 CompletedConfig）
	completedConfig, err := validConfig.Complete()
	if err != nil {
		fmt.Printf("配置无效：%v\n", err)
		return
	}
	// 步骤2：用 CompletedConfig 创建服务器（合法）
	server := completedConfig.NewServer()

	fmt.Printf("服务器启动成功（模式：%s，端口：%d）\n", server.mode, server.port)
}
