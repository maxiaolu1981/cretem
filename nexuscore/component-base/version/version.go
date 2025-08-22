// Package version
// version.go
//
//	提供程序版本信息的管理功能，包括Git版本、构建时间、提交哈希等元数据的存储和格式化输出，
//
// 支持以文本表格、JSON等形式展示版本信息，方便用户和系统查看程序的构建来源和环境。
package version

import (
	"fmt"
	"os"
	"runtime"

	"github.com/gosuri/uitable" // 用于创建格式化的表格输出

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/json" // 自定义JSON序列化工具
)

var (
	// GitVersion 语义化版本号，如 v1.2.3，通常通过编译参数注入
	GitVersion = "v0.0.0-master+$Format:%h$"
	// BuildDate 构建时间，ISO8601格式（如 2024-05-20T15:30:00Z），通常通过编译参数注入
	BuildDate = "1970-01-01T00:00:00Z"
	// GitCommit Git提交哈希，完整的SHA-1值，通常通过编译参数注入
	GitCommit = "$Format:%H$"
	// GitTreeState Git工作区状态，"clean"表示无未提交修改，"dirty"表示有未提交修改
	GitTreeState = ""
)

// Info 包含程序版本相关的所有元数据
type Info struct {
	GitVersion   string `json:"gitVersion"`   // 语义化版本号
	GitCommit    string `json:"gitCommit"`    // Git提交哈希
	GitTreeState string `json:"gitTreeState"` // Git工作区状态
	BuildDate    string `json:"buildDate"`    // 构建时间
	GoVersion    string `json:"goVersion"`    // 编译使用的Go版本
	Compiler     string `json:"compiler"`     // 编译器名称（如 gc）
	Platform     string `json:"platform"`     // 目标运行平台（如 linux/amd64）
}

// String 以人类友好的文本格式返回版本信息，优先使用表格格式，失败时返回Git版本号
func (info Info) String() string {
	if s, err := info.Text(); err == nil {
		return string(s)
	}

	return info.GitVersion
}

// ToJSON 将版本信息序列化为JSON字符串
func (info Info) ToJSON() string {
	s, _ := json.Marshal(info) // 忽略错误，确保返回非空字符串

	return string(s)
}

// Text 将版本信息格式化为UTF-8编码的表格文本并返回
func (info Info) Text() ([]byte, error) {
	table := uitable.New() // 创建新表格
	table.RightAlign(0)    // 第一列右对齐
	table.MaxColWidth = 80 // 最大列宽限制为80字符
	table.Separator = " "  // 列分隔符为空格
	// 向表格添加版本信息行
	table.AddRow("gitVersion:", info.GitVersion)
	table.AddRow("gitCommit:", info.GitCommit)
	table.AddRow("gitTreeState:", info.GitTreeState)
	table.AddRow("buildDate:", info.BuildDate)
	table.AddRow("goVersion:", info.GoVersion)
	table.AddRow("compiler:", info.Compiler)
	table.AddRow("platform:", info.Platform)

	return table.Bytes(), nil // 返回表格的字节形式
}

// Get 获取程序的完整版本信息，整合预定义的元数据和运行时信息
// 这些变量通常通过编译时的-ldflags参数注入，若未注入则使用默认值
func Get() Info {
	return Info{
		GitVersion:   GitVersion,
		GitCommit:    GitCommit,
		GitTreeState: GitTreeState,
		BuildDate:    BuildDate,
		GoVersion:    runtime.Version(),                                  // 从Go运行时获取编译版本（如 go1.21.0）
		Compiler:     runtime.Compiler,                                   // 从Go运行时获取编译器（通常为 "gc"）
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), // 构建目标平台（如 linux/amd64）
	}
}

func CheckVersionAndExit() {
	for _, arg := range os.Args {
		switch {
		case arg == "--version" || arg == "--version=true":
			version, _ := Get().Text()
			println(string(version))
			os.Exit(0)
		case arg == "--version=raw":
			println(Get().ToJSON()) // 输出JSON格式
			os.Exit(0)
		}
	}
}
