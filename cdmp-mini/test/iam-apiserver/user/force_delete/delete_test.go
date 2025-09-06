package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// ==================== 全局配置（必须修改！） ====================
var (
	apiBaseURL   = "http://127.0.0.1:8080/v1/users"                                                                                                                                                                                                                                                                                   // 接口地址
	token        = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJodHRwczovL2dpdGh1Yi5jb20vbWF4aWFvbHUxOTgxL2NyZXRlbSIsImV4cCI6MTc1NzE1Nzg0MCwiaWRlbnRpdHkiOiJhZG1pbiIsImlzcyI6Imh0dHBzOi8vZ2l0aHViLmNvbS9tYXhpYW9sdTE5ODEvY3JldGVtIiwib3JpZ19pYXQiOjE3NTcwNzE0NDAsInN1YiI6ImFkbWluIn0.1nXWXRqevSUD8TDNEdksXaexQEZZkAd47V2uUGV4AA4" // 有效令牌
	validUser    = "forbiddenuser1757014079"                                                                                                                                                                                                                                                                                          // 确保存在的用户（204测试）
	nonExistUser = "non_exist_" + fmt.Sprintf("%d", time.Now().Unix())                                                                                                                                                                                                                                                                // 不存在的用户（404测试）
	invalidUser  = "invalid@user"                                                                                                                                                                                                                                                                                                     // 格式无效的用户（400测试）
	timeout      = 10 * time.Second                                                                                                                                                                                                                                                                                                   // 超时时间

	// 原生ANSI颜色码
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32;1m"
	ansiRed    = "\033[31;1m"
	ansiBlue   = "\033[34m"
	ansiPurple = "\033[35m"
	ansiYellow = "\033[33;1m"

	// 测试统计
	total  int
	passed int
	failed int
)

// ==================== 工具函数 ====================
// 发送DELETE请求
func sendDeleteRequest(url string, headers map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置默认头
	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("发送请求失败: %w", err)
	}

	// 读取响应体
	defer resp.Body.Close()
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	return resp, string(body[:n]), nil
}

// 测试用例执行函数
func runTestCase(t *testing.T, testName, url string, expectedCode int, headers map[string]string) {
	total++
	fmt.Printf("\n%s用例 %d: %s%s\n", ansiYellow, total, testName, ansiReset)
	fmt.Println("----------------------------------------")
	fmt.Printf("%s描述: 预期状态码 %d，请求地址: %s%s\n", ansiBlue, expectedCode, url, ansiReset)

	// 发送请求
	resp, body, err := sendDeleteRequest(url, headers)
	if err != nil {
		fmt.Printf("%s❌ 发送请求失败: %v%s\n", ansiRed, err, ansiReset)
		failed++
		t.Fatalf("用例「%s」执行失败: %v", testName, err)
	}

	// 验证状态码
	actualCode := resp.StatusCode
	casePassed := actualCode == expectedCode

	// 输出结果
	if casePassed {
		fmt.Printf("%s✅ 状态码正确: 预期 %d, 实际 %d%s\n", ansiGreen, expectedCode, actualCode, ansiReset)
	} else {
		fmt.Printf("%s❌ 状态码错误: 预期 %d, 实际 %d%s\n", ansiRed, expectedCode, actualCode, ansiReset)
	}

	// 输出响应体
	fmt.Printf("%s响应体: %s%s\n", ansiBlue, body, ansiReset)

	// 统计结果
	if casePassed {
		fmt.Printf("%s----------------------------------------%s\n", ansiGreen, ansiReset)
		fmt.Printf("%s✅ 用例执行通过 %s\n", ansiGreen, ansiReset)
		passed++
	} else {
		fmt.Printf("%s----------------------------------------%s\n", ansiRed, ansiReset)
		fmt.Printf("%s❌ 用例执行失败 %s\n", ansiRed, ansiReset)
		failed++
		t.Errorf("用例「%s」失败: 状态码不匹配", testName)
	}
}

// ==================== 测试用例实现 ====================
// 1. 400参数错误（用户名为空）
func case400EmptyUser(t *testing.T) {
	url := fmt.Sprintf("%s//force", apiBaseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	runTestCase(t, "400参数错误（用户名为空）", url, http.StatusBadRequest, headers)
}

// 2. 400参数错误（用户名含特殊字符@）
func case400InvalidChar(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, invalidUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	runTestCase(t, "400参数错误（用户名含@）", url, http.StatusBadRequest, headers)
}

// 3. 401未授权（无令牌）
func case401NoToken(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{} // 无令牌
	runTestCase(t, "401未授权（无令牌）", url, http.StatusUnauthorized, headers)
}

// 4. 401未授权（无效令牌）
func case401InvalidToken(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{
		"Authorization": "Bearer invalid_token_xxx",
	}
	runTestCase(t, "401未授权（无效令牌）", url, http.StatusUnauthorized, headers)
}

// 5. 404用户不存在
func case404UserNotFound(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, nonExistUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	runTestCase(t, "404用户不存在", url, http.StatusNotFound, headers)
}

// 6. 204删除成功（有效用户）
func case204DeleteSuccess(t *testing.T) {
	url := fmt.Sprintf("%s/%s/force", apiBaseURL, validUser)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	runTestCase(t, "204删除成功（有效用户）", url, http.StatusNoContent, headers)
}

// 7. 500服务器内部错误（需后端配合）
func case500InternalError(t *testing.T) {
	url := fmt.Sprintf("%s/trigger_500/force", apiBaseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	runTestCase(t, "500服务器内部错误", url, http.StatusInternalServerError, headers)
}

// ==================== 测试入口 ====================
func TestDeleteUser_AllCases(t *testing.T) {
	// 验证配置
	if token == "" || validUser == "" {
		fmt.Printf("%s❌ 请先配置有效的token和validUser！%s\n", ansiRed, ansiReset)
		t.Fatal("配置不完整，测试终止")
	}

	// 开始提示
	fmt.Println("================================================================================")
	fmt.Printf("%s开始执行用户硬删除接口测试（%s）%s\n", ansiBlue, time.Now().Format(time.RFC3339), ansiReset)
	fmt.Println("================================================================================")

	// 初始化统计
	total, passed, failed = 0, 0, 0

	// 执行测试用例
	t.Run("用例1_400空用户名", func(t *testing.T) { case400EmptyUser(t) })
	t.Run("用例2_400特殊字符", func(t *testing.T) { case400InvalidChar(t) })
	t.Run("用例3_401无令牌", func(t *testing.T) { case401NoToken(t) })
	t.Run("用例4_401无效令牌", func(t *testing.T) { case401InvalidToken(t) })
	t.Run("用例5_404不存在用户", func(t *testing.T) { case404UserNotFound(t) })
	t.Run("用例6_204删除成功", func(t *testing.T) { case204DeleteSuccess(t) })
	// 如需测试500，取消下面一行注释（需后端配合）
	// t.Run("用例7_500服务器错误", func(t *testing.T) { case500InternalError(t) })

	// 测试总结
	fmt.Println("\n================================================================================")
	fmt.Printf("测试总结: 总用例数: %d, 通过: %d, 失败: %d", total, passed, failed)
	fmt.Println("================================================================================")

	// 输出结果
	if failed > 0 {
		fmt.Printf("%s❌ 存在%d个失败用例，请检查：%s\n", ansiRed, failed, ansiReset)
		fmt.Printf("%s1. 配置区的VALID_USER是否真的存在？%s\n", ansiRed, ansiReset)
		fmt.Printf("%s2. 后端接口是否正确返回对应状态码？%s\n", ansiRed, ansiReset)
		fmt.Printf("%s3. 令牌是否过期（可重新获取TOKEN）？%s\n", ansiRed, ansiReset)
		t.Fatalf("共有 %d 个用例失败", failed)
	} else {
		fmt.Printf("%s🎉 所有测试用例全部通过!%s\n", ansiGreen, ansiReset)
	}
}
