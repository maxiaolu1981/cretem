package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cretem/nexuscore/log" // 替换为实际日志库路径
)

func main1() {
	// 1. 验证程序启动
	fmt.Println("=== 程序启动，开始测试日志输出 ===")

	// 2. 配置日志（使用绝对路径）
	logPath := "/tmp/cdmp-logs/app.log"
	errorLogPath := "/tmp/cdmp-logs/error.log"

	// 3. 初始化日志目录（带权限检查）
	if err := initLogDir(logPath); err != nil {
		fmt.Printf("初始化日志目录失败: %v\n", err)
		return
	}
	fmt.Printf("日志目录验证通过，日志将写入: %s\n", logPath)

	// 4. 初始化日志库
	logOpts := &log.Options{
		Level:            "debug",   // 最低级别，确保所有日志都能输出
		Format:           "console", // 控制台格式，便于查看
		EnableColor:      false,
		EnableCaller:     true,
		OutputPaths:      []string{logPath, "stdout"}, // 同时输出到文件和控制台
		ErrorOutputPaths: []string{errorLogPath, "stderr"},
	}
	log.Init(logOpts)

	// 5. 检查日志器是否初始化
	if log.GetLogger() == nil {
		fmt.Println("错误：日志库初始化失败，全局日志器为nil")
		return
	}
	fmt.Println("日志库初始化成功，开始输出测试日志...")

	// 6. 输出不同级别的日志
	testLogger := log.WithName("test").WithValues("test_id", "123")

	// 强制输出Error级别（最高优先级）
	testLogger.Error(errors.New("test error"), "这是一条错误日志")

	// 输出Info级别
	testLogger.Info("这是一条信息日志", "key", "value")

	// 输出调试级别
	testLogger.V(1).Info("这是一条调试日志", "debug_key", "debug_value")

	// 7. 立即刷新日志缓冲区
	log.Flush()
	fmt.Println("=== 测试日志输出完成，请检查日志文件 ===")

	// 8. 等待3秒后退出（确保日志写入）
	// time.Sleep(3 * time.Second)
}

// 带权限检查的日志目录初始化
func initLogDir1(logPath string) error {
	dir := filepath.Dir(logPath)
	// 创建目录（若不存在）
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	// 检查目录可写性
	testFile := filepath.Join(dir, "write_test.tmp")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("目录 %s 不可写: %w", dir, err)
	}
	defer func() {
		f.Close()
		os.Remove(testFile)
	}()
	return nil
}
