package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func main() {
	fmt.Println("🔍 详细接口诊断")
	fmt.Println("======================")

	// 测试各个接口响应时间
	testEndpoint("healthz", "GET", "", nil)
	testEndpoint("login", "POST", `{"username":"admin","password":"Admin@2021"}`, nil)

	// 先登录获取token
	token := getAuthToken()
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		testEndpoint("v1/users/admin", "GET", "", headers)
		testEndpoint("v1/users/nonexistent", "GET", "", headers) // 测试404
	}

	// 检查服务器资源使用
	checkServerResources()
}

func testEndpoint(path, method, body string, headers map[string]string) {
	fmt.Printf("\n🔧 测试 %s %s: ", method, path)

	client := &http.Client{Timeout: 30 * time.Second}
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
		fmt.Printf("❌ 创建请求失败: %v\n", err)
		return
	}

	// 设置headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if duration > 2*time.Second {
		fmt.Printf("⚠️  过慢: %v ", duration)
	} else {
		fmt.Printf("✅ 正常: %v ", duration)
	}

	fmt.Printf("状态码: %d\n", resp.StatusCode)

	// 显示部分响应内容
	if len(respBody) > 200 {
		fmt.Printf("   响应: %s...\n", string(respBody[:200]))
	} else {
		fmt.Printf("   响应: %s\n", string(respBody))
	}
}

func getAuthToken() string {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("http://localhost:8088/login",
		"application/json",
		strings.NewReader(`{"username":"admin","password":"Admin@2021"}`))
	if err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if token, ok := result["access_token"].(string); ok {
		return token
	}

	fmt.Printf("❌ 无法解析token: %s\n", string(body))
	return ""
}

func checkServerResources() {
	fmt.Println("\n📊 服务器资源状态:")

	// 检查进程打开文件数
	cmd := exec.Command("sh", "-c", "lsof -p $(lsof -ti:8088 | head -1) 2>/dev/null | wc -l")
	output, _ := cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("打开文件数: %s", output)
	}

	// 检查内存
	cmd = exec.Command("sh", "-c", "ps -o pid,ppid,rss,vsz,pcpu,pmem,command -p $(lsof -ti:8088 | head -1) 2>/dev/null")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("进程资源:\n%s\n", output)
	}
}
