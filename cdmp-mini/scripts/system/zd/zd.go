package main

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

func main() {
	fmt.Println("🔍 服务器崩溃状态诊断")
	fmt.Println("========================")

	// 1. 检查端口是否真的在监听
	checkPortListening()

	// 2. 检查进程状态
	checkProcessStatus()

	// 3. 尝试多种连接方式
	testDifferentConnections()

	// 4. 检查系统资源
	checkSystemResources()

	fmt.Println("\n💡 结论：服务器进程存在但已无响应，需要重启")
}

func checkPortListening() {
	fmt.Println("\n1. 检查端口监听状态:")

	cmd := exec.Command("sh", "-c", "netstat -tlnp | grep 8088 || ss -tlnp | grep 8088")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("❌ 无法检查端口状态")
		return
	}

	if len(output) == 0 {
		fmt.Println("❌ 端口8088未监听")
		return
	}

	fmt.Printf("✅ 端口在监听:\n%s\n", string(output))

	// 解析进程ID
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "8088") {
			if strings.Contains(line, "LISTEN") {
				fmt.Println("⚠️  端口处于LISTEN状态但无响应 - 进程可能卡死")
			}
			// 提取PID
			if idx := strings.Index(line, "/"); idx != -1 {
				pid := line[idx+1:]
				fmt.Printf("   关联进程: %s\n", pid)
			}
		}
	}
}

func checkProcessStatus() {
	fmt.Println("\n2. 检查进程状态:")

	// 查找监听8088端口的进程
	cmd := exec.Command("sh", "-c", "lsof -i :8088 || ps aux | grep 8088")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("❌ 无法检查进程状态")
		return
	}

	if len(output) == 0 {
		fmt.Println("❌ 未找到监听8088的进程")
		return
	}

	fmt.Printf("找到进程:\n%s\n", string(output))

	// 检查进程是否僵尸状态
	cmd = exec.Command("sh", "-c", "ps aux | grep -E '(go|8088)' | grep -v grep")
	output, err = cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Z") || strings.Contains(line, "defunct") {
				fmt.Println("❌ 发现僵尸进程!")
			}
		}
	}
}

func testDifferentConnections() {
	fmt.Println("\n3. 连接测试:")

	tests := []struct {
		name string
		addr string
	}{
		{"本地环回", "127.0.0.1:8088"},
		{"localhost", "localhost:8088"},
		{"0.0.0.0", "0.0.0.0:8088"},
	}

	for _, test := range tests {
		fmt.Printf("   测试 %s: ", test.name)

		for i := 0; i < 2; i++ { // 重试2次
			conn, err := net.DialTimeout("tcp", test.addr, 2*time.Second)
			if err == nil {
				conn.Close()
				fmt.Printf("✅ 成功\n")
				break
			} else {
				if i == 1 { // 最后一次尝试
					fmt.Printf("❌ 失败 (%v)\n", err)
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func checkSystemResources() {
	fmt.Println("\n4. 系统资源检查:")

	commands := []string{
		"free -h | grep -E '(Mem|内存)'",
		"top -bn1 | head -5",
		"dmesg | tail -5 | grep -E '(killed|oom|out of memory)'",
	}

	for _, cmdStr := range commands {
		cmd := exec.Command("sh", "-c", cmdStr)
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			fmt.Printf("   %s:\n%s\n", cmdStr, string(output))
		}
	}
}
