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
该包定义了 API 服务器的功能开关配置选项（FeatureOptions），包含性能分析（profiling）和指标收集（metrics）的启用状态，提供配置初始化、与服务器核心配置的映射、命令行参数绑定及（预留）合法性校验功能，用于控制服务器的附加功能是否启用。
函数流程详解
FeatureOptions 结构体
存储服务器附加功能的开关状态，主要包含：
EnableProfiling：是否启用性能分析功能（通过 /debug/pprof/ 路径暴露性能数据）。
EnableMetrics：是否启用指标收集功能（通过 /metrics 路径暴露监控指标）。
NewFeatureOptions 函数
功能：创建带有默认值的 FeatureOptions 实例。
流程：通过 server.NewConfig() 获取服务器默认配置，将默认的功能开关状态（EnableProfiling 和 EnableMetrics）赋值给新实例，确保初始配置与服务器默认行为一致（通常默认启用这两个功能）。
ApplyTo 函数
功能：将当前功能开关配置映射到服务器核心配置（server.Config）。
流程：直接将 EnableProfiling 和 EnableMetrics 的值赋值给 server.Config 的对应字段，使服务器运行时启用或禁用相应功能。
Validate 函数
功能：校验功能开关配置的合法性（当前为预留接口，未实现具体逻辑）。
潜在扩展：未来若功能开关有依赖关系（如某功能必须依赖另一功能），可在此添加校验逻辑。
AddFlags 函数
功能：为命令行解析绑定功能开关相关参数，允许通过命令行覆盖默认配置。
流程：
绑定 --feature.profiling：控制是否启用性能分析功能，说明通过 /debug/pprof/ 路径访问。
绑定 --feature.enable-metrics：控制是否启用指标收集功能，说明通过 /metrics 路径访问。
*/
// options 包定义了 API 服务器的功能开关配置选项，包含性能分析和指标收集的启用状态，
// 提供配置初始化、服务器核心配置映射、命令行参数绑定及合法性校验（预留）功能。
package options

import (
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/pkg/server"
	"github.com/spf13/pflag"
)

// FeatureOptions 包含与 API 服务器功能相关的配置项（功能开关）
type FeatureOptions struct {
	EnableProfiling bool `json:"profiling"      mapstructure:"profiling"`      // 是否启用性能分析功能
	EnableMetrics   bool `json:"enable-metrics" mapstructure:"enable-metrics"` // 是否启用指标收集功能
}

// NewFeatureOptions 创建带有默认参数的 FeatureOptions 实例
func NewFeatureOptions() *FeatureOptions {
	defaults := server.NewConfig() // 获取服务器默认配置

	return &FeatureOptions{
		EnableMetrics:   defaults.EnableMetrics,   // 继承默认指标收集开关状态
		EnableProfiling: defaults.EnableProfiling, // 继承默认性能分析开关状态
	}
}

// ApplyTo 将当前功能开关配置应用到服务器核心配置（server.Config）
func (o *FeatureOptions) ApplyTo(c *server.Config) error {
	c.EnableProfiling = o.EnableProfiling // 覆盖性能分析开关
	c.EnableMetrics = o.EnableMetrics     // 覆盖指标收集开关

	return nil
}

// Validate 解析并校验程序启动时用户通过命令行输入的参数（当前未实现具体逻辑）
func (o *FeatureOptions) Validate() []error {
	return []error{} // 可扩展：添加功能开关的依赖关系校验等
}

// AddFlags 为特定 API 服务器添加与功能开关相关的命令行参数到指定的 FlagSet
func (o *FeatureOptions) AddFlags(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}

	// 绑定 --feature.profiling 参数：控制是否启用性能分析
	fs.BoolVar(&o.EnableProfiling, "feature.profiling", o.EnableProfiling,
		"通过 web 接口 host:port/debug/pprof/ 启用性能分析。")

	// 绑定 --feature.enable-metrics 参数：控制是否启用指标收集
	fs.BoolVar(&o.EnableMetrics, "feature.enable-metrics", o.EnableMetrics,
		"在 API 服务器上启用 /metrics 路径的指标收集功能。")
}
