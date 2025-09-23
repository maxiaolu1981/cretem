package main

import (
	"fmt"
	"os/exec"
)

func main() {
	fmt.Println("🔍 分析服务器崩溃原因")

	// 检查是否是测试导致的
	fmt.Println("\n1. 检查测试相关进程:")
	cmd := exec.Command("sh", "-c", "ps aux | grep -E '(test|go.test)' | grep -v grep")
	output, _ := cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("发现测试进程:\n%s\n", string(output))
		fmt.Println("💡 可能是之前的测试未正确清理")
	}

	// 检查文件描述符
	fmt.Println("\n2. 检查文件描述符:")
	cmd = exec.Command("sh", "-c", "lsof -p $(lsof -ti:8088) 2>/dev/null | wc -l")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("进程打开文件数: %s", output)
	}

	// 检查内存使用历史
	fmt.Println("\n3. 检查系统日志:")
	cmd = exec.Command("sh", "-c", "dmesg | tail -10 | grep -E '(Memory|killed|panic)'")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("系统日志:\n%s\n", string(output))
	}

	fmt.Println("\n🎯 根本原因可能是:")
	fmt.Println("   - 测试未正确释放资源（数据库连接、HTTP连接）")
	fmt.Println("   - 内存泄漏导致OOM Killer杀死进程")
	fmt.Println("   - 死锁或goroutine泄漏")
	fmt.Println("   - 文件描述符耗尽")
}
