package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// 服务器状态诊断
func main() {
	fmt.Println("🔍 开始服务器状态诊断...")
	fmt.Println("==================================")

	// 1. 检查Go运行时状态
	checkGoRuntimeStatus()

	// 2. 检查服务器健康状态
	checkServerHealth()

	// 3. 简单压力测试（小规模）
	quickStressTest()

	// 4. 检查恢复能力
	checkRecoveryAbility()

	fmt.Println("==================================")
	fmt.Println("✅ 诊断完成")
}

// 检查Go运行时状态
func checkGoRuntimeStatus() {
	fmt.Println("\n📊 Go运行时状态:")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Printf("Goroutines数量: %d\n", runtime.NumGoroutine())
	fmt.Printf("内存使用: Alloc=%.1fMB, Sys=%.1fMB\n",
		float64(m.Alloc)/1024/1024, float64(m.Sys)/1024/1024)
	fmt.Printf("GC统计: 次数=%d, 总暂停时间=%v\n", m.NumGC, time.Duration(m.PauseTotalNs))
	fmt.Printf("堆内存: 使用=%.1fMB, 系统=%.1fMB\n",
		float64(m.HeapInuse)/1024/1024, float64(m.HeapSys)/1024/1024)

	// 判断是否正常
	if runtime.NumGoroutine() > 1000 {
		fmt.Println("❌ Goroutine泄漏嫌疑")
	} else {
		fmt.Println("✅ Goroutine数量正常")
	}

	if m.Alloc > 500*1024*1024 { // 500MB
		fmt.Println("❌ 内存占用过高")
	} else {
		fmt.Println("✅ 内存使用正常")
	}
}

// 检查服务器健康状态
func checkServerHealth() {
	fmt.Println("\n🌐 服务器健康检查:")

	client := &http.Client{Timeout: 10 * time.Second}

	// 测试登录接口
	start := time.Now()
	resp, err := client.Post("http://localhost:8088/login",
		"application/json",
		strings.NewReader(`{"username":"admin","password":"Admin@2021"}`))
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ 登录接口不可达: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("登录接口: 状态码=%d, 响应时间=%v\n", resp.StatusCode, duration)

	if resp.StatusCode != 200 {
		fmt.Printf("❌ 登录失败: %s\n", string(body))
	} else {
		fmt.Println("✅ 登录接口正常")
	}

	// 测试用户查询接口（使用热点用户）
	testUserQuery(client, "admin", "热点用户")
	testUserQuery(client, "nonexistent-user-123", "无效用户")
}

func testUserQuery(client *http.Client, userID, testType string) {
	url := fmt.Sprintf("http://localhost:8088/v1/users/%s", userID)

	// 先登录获取token
	token, err := getAuthToken(client)
	if err != nil {
		fmt.Printf("❌ 获取token失败: %v\n", err)
		return
	}

	start := time.Now()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ %s查询失败: %v\n", testType, err)
		return
	}
	defer resp.Body.Close()

	_, _ = io.ReadAll(resp.Body)
	fmt.Printf("%s查询: 状态码=%d, 耗时=%v\n", testType, resp.StatusCode, duration)

	if duration > 2*time.Second {
		fmt.Printf("⚠️  %s查询过慢: %v\n", testType, duration)
	}
}

func getAuthToken(client *http.Client) (string, error) {
	resp, err := client.Post("http://localhost:8088/login",
		"application/json",
		strings.NewReader(`{"username":"admin","password":"Admin@2021"}`))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if token, ok := result["access_token"].(string); ok {
		return token, nil
	}

	return "", fmt.Errorf("无法获取token")
}

// 快速压力测试
func quickStressTest() {
	fmt.Println("\n⚡ 快速压力测试(10并发 x 5请求):")

	token, err := getAuthToken(&http.Client{Timeout: 10 * time.Second})
	if err != nil {
		fmt.Printf("❌ 获取token失败: %v\n", err)
		return
	}

	success := 0
	totalDuration := time.Duration(0)
	client := &http.Client{Timeout: 30 * time.Second}

	// 10个并发，每个5次请求
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 5; j++ {
				start := time.Now()
				url := fmt.Sprintf("http://localhost:8088/v1/users/user_%d_%d_test", id, j)

				req, _ := http.NewRequest("GET", url, nil)
				req.Header.Set("Authorization", "Bearer "+token)

				resp, err := client.Do(req)
				duration := time.Since(start)

				if err == nil && resp.StatusCode == 200 {
					success++
					totalDuration += duration
					resp.Body.Close()
				}

				time.Sleep(100 * time.Millisecond) // 稍微间隔
			}
		}(i)
	}

	time.Sleep(3 * time.Second) // 等待测试完成

	if success > 0 {
		avgTime := totalDuration / time.Duration(success)
		fmt.Printf("结果: 成功%d/50, 平均响应时间=%v\n", success, avgTime)

		if avgTime > 500*time.Millisecond {
			fmt.Println("❌ 平均响应时间过长")
		}
	}
}

// 检查恢复能力
func checkRecoveryAbility() {
	fmt.Println("\n🔄 恢复能力检查:")

	// 强制GC
	runtime.GC()
	time.Sleep(1 * time.Second)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Printf("GC后内存: %.1fMB, Goroutines: %d\n",
		float64(m.Alloc)/1024/1024, runtime.NumGoroutine())

	if runtime.NumGoroutine() < 100 {
		fmt.Println("✅ 恢复能力正常")
	} else {
		fmt.Println("❌ 可能存在资源泄漏")
	}
}
