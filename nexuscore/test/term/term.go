package main

import (
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/term"
)

func main() {
	// 1. 尝试获取标准输出（stdout）的终端尺寸
	width, height, err := term.TerminalSize(os.Stdout)
	if err != nil {
		fmt.Printf("无法获取终端尺寸: %v\n", err)
		// 输出到非终端设备时（如重定向到文件），使用默认尺寸
		width = 80
		height = 24
		fmt.Printf("使用默认终端尺寸: 宽 %d, 高 %d\n", width, height)
	} else {
		fmt.Printf("当前终端尺寸: 宽 %d 列, 高 %d 行\n", width, height)
	}

	// 2. 根据终端宽度动态调整输出内容
	demoDynamicOutput(width)

	// 3. 演示对非终端设备的判断（如标准错误stderr）
	_, _, err = term.TerminalSize(os.Stderr)
	if err != nil {
		fmt.Printf("检查stderr是否为终端: %v\n", err)
	} else {
		fmt.Println("stderr是一个终端设备")
	}
}

// 根据终端宽度动态调整输出格式
func demoDynamicOutput(terminalWidth int) {
	fmt.Println("\n=== 动态适应终端宽度的输出 ===")

	// 生成一段示例文本
	content := "这是一段用于演示终端宽度适应的文本，会根据终端宽度自动换行或截断。"

	// 根据终端宽度调整显示方式
	if terminalWidth <= 40 {
		// 窄终端：每行最多显示20个字符
		fmt.Println("检测到窄终端，启用紧凑显示模式：")
		printWrapped(content, 20)
	} else if terminalWidth <= 80 {
		// 中等宽度终端：每行最多显示40个字符
		fmt.Println("检测到中等宽度终端：")
		printWrapped(content, 40)
	} else {
		// 宽终端：每行最多显示60个字符
		fmt.Println("检测到宽终端，启用宽松显示模式：")
		printWrapped(content, 60)
	}
}

// 按指定宽度换行输出文本
func printWrapped(text string, lineWidth int) {
	// 将文本按指定宽度拆分并输出
	start := 0
	for start < len(text) {
		end := start + lineWidth
		if end > len(text) {
			end = len(text)
		}
		fmt.Println(text[start:end])
		start = end
	}
}
