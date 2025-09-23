package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func main() {
	fmt.Println("🔍 应用层深度诊断")

	// 测试绕过认证的接口
	testPublicEndpoints()

	// 测试认证接口的不同情况
	testAuthScenarios()

	// 分析可能的问题点
	analyzeApplicationIssue()
}

func testPublicEndpoints() {
	fmt.Println("\n1. 测试公开接口:")

	endpoints := []string{
		"healthz",
		"metrics",
		"version",
		"api/info",
	}

	for _, endpoint := range endpoints {
		testURL := "http://localhost:8088/" + endpoint
		fmt.Printf("测试 %s: ", endpoint)

		client := &http.Client{Timeout: 3 * time.Second}
		start := time.Now()
		resp, err := client.Get(testURL)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("❌ 失败: %v\n", err)
		} else {
			resp.Body.Close()
			if duration > time.Second {
				fmt.Printf("⚠️  慢: %v 状态: %d\n", duration, resp.StatusCode)
			} else {
				fmt.Printf("✅ 正常: %v 状态: %d\n", duration, resp.StatusCode)
			}
		}
	}
}

func testAuthScenarios() {
	fmt.Println("\n2. 测试认证相关:")

	// 测试1: 不带认证头的用户查询
	fmt.Printf("测试 无认证用户查询: ")
	testEndpoint("v1/users/admin", "GET", "", nil)

	// 测试2: 错误格式的认证头
	fmt.Printf("测试 错误认证头: ")
	headers := map[string]string{"Authorization": "InvalidToken"}
	testEndpoint("v1/users/admin", "GET", "", headers)

	// 测试3: 空的登录请求
	fmt.Printf("测试 空登录请求: ")
	testEndpoint("login", "POST", "{}", nil)

	// 测试4: 错误密码登录
	fmt.Printf("测试 错误密码: ")
	testEndpoint("login", "POST", `{"username":"admin","password":"wrong"}`, nil)
}

func testEndpoint(path, method, body string, headers map[string]string) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://localhost:8088/" + path

	var req *http.Request
	var err error

	if body != "" {
		req, err = http.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		fmt.Printf("❌ 创建请求失败\n")
		return
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ 超时/失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if duration > 2*time.Second {
		fmt.Printf("⚠️  过慢: %v 状态: %d\n", duration, resp.StatusCode)
	} else {
		fmt.Printf("✅ 正常: %v 状态: %d\n", duration, resp.StatusCode)
	}
}

func analyzeApplicationIssue() {
	fmt.Println("\n3. 问题分析:")
	fmt.Println("📍 可能的问题点:")
	fmt.Println("   • 认证中间件死锁")
	fmt.Println("   • 密码验证服务卡死")
	fmt.Println("   • JWT生成逻辑阻塞")
	fmt.Println("   • 数据库连接池配置错误")
	fmt.Println("   • HTTP路由配置问题")

	fmt.Println("\n🎯 解决方案:")
	fmt.Println("   1. 重启应用服务")
	fmt.Println("   2. 检查应用日志中的panic或deadlock")
	fmt.Println("   3. 检查认证相关中间件配置")
	fmt.Println("   4. 验证密码验证逻辑")
}
