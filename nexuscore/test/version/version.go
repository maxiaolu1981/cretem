package main

import (
	"fmt"
	"os"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/version"
)

func main() {
	// 1. 获取完整版本信息（调用Get()函数）
	info := version.Get()
	fmt.Println("=== 基本版本信息 ===")
	fmt.Printf("当前版本: %s\n", info.GitVersion)
	fmt.Printf("构建平台: %s\n", info.Platform)
	fmt.Printf("Go版本: %s\n", info.GoVersion)

	// 2. 使用String()方法输出人类友好的表格格式
	fmt.Println("\n=== String() 表格格式输出 ===")
	fmt.Println(info.String())

	// 3. 使用ToJSON()方法输出JSON格式
	fmt.Println("\n=== ToJSON() JSON格式输出 ===")
	fmt.Println(info.ToJSON())

	// 4. 使用Text()方法获取原始表格字节数据（可用于写入文件）
	fmt.Println("\n=== Text() 原始表格字节数据 ===")
	text, err := info.Text()
	if err != nil {
		fmt.Printf("获取文本格式失败: %v\n", err)
		os.Exit(1)
	}
	// 直接打印字节数据（等价于String()的内部实现）
	fmt.Println(string(text))

	// 5. 实际应用：将版本信息写入文件
	err = writeVersionToFile(info, "version.txt")
	if err != nil {
		fmt.Printf("写入版本文件失败: %v\n", err)
	} else {
		fmt.Println("\n版本信息已写入 version.txt")
	}
}

// 演示如何将版本信息写入文件（支持不同格式）
func writeVersionToFile(info version.Info, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// 写入表格格式
	_, err = file.WriteString("=== 程序版本信息 ===\n")
	if err != nil {
		return err
	}

	text, err := info.Text()
	if err != nil {
		return err
	}
	_, err = file.Write(text)
	if err != nil {
		return err
	}

	// 追加JSON格式
	_, err = file.WriteString("\n\n=== JSON格式版本信息 ===\n")
	if err != nil {
		return err
	}
	_, err = file.WriteString(info.ToJSON())
	return err
}
