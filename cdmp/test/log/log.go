package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/log"
	"github.com/robfig/cron/v3"
)

// 初始化日志目录（确保可写）
func initLogDir(logPath string) error {
	dir := filepath.Dir(logPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}
	// 验证目录可写性
	testFile := filepath.Join(dir, "write_test.tmp")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("日志目录不可写: %w", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

func main() {
	// 1. 初始化日志配置
	logOpts := log.NewOptions()
	logOpts.Format = "console" // 显式指定控制台格式
	logOpts.Level = "info"     // 确保info级别日志输出

	// 2. 提前初始化日志目录
	if len(logOpts.OutputPaths) > 0 {
		if err := initLogDir(logOpts.OutputPaths[0]); err != nil {
			fmt.Printf("日志目录初始化失败: %v\n", err)
			return
		}
	}
	if len(logOpts.ErrorOutputPaths) > 0 {
		if err := initLogDir(logOpts.ErrorOutputPaths[0]); err != nil {
			fmt.Printf("错误日志目录初始化失败: %v\n", err)
			return
		}
	}

	// 3. 初始化日志库
	log.Init(logOpts)
	defer log.Flush()

	// 4. 检查日志器是否初始化成功
	if log.GetLogger() == nil {
		fmt.Println("日志库初始化失败，无法输出日志")
		return
	}

	// 5. 手动触发一条错误日志（第1条error日志）
	fmt.Println("=== 手动触发测试错误日志 ===")
	log.GetLogger().Error(
		errors.New("manual test error"),
		"这是一条测试错误日志，应同时出现在error.log和控制台",
		"source", "main",
	)
	log.GetLogger().Info("info格式", "username", "password")
	log.GetLogger().Infof("this is a formatted messsage username:%s passwd:%d", "mxl", 10)
	log.V(0).Info("this is a v level message")
	log.Flush()

	// 6. 触发业务逻辑错误
	fmt.Println("=== 触发业务逻辑错误 ===")
	serverLogger := log.WithName("server")

	// 数据库连接错误（第1条info日志，第2条error日志）
	dbLogger := serverLogger.WithName("db")
	dbLogger.Info("尝试连接数据库（预期失败）") // info日志1
	if err := connectDB(); err != nil {
		dbLogger.Error(err, "数据库连接失败（业务错误）", "retry", 3) // error日志2
	}

	// 启动定时任务（修复返回值问题）
	cronLogger := serverLogger.WithName("cron")
	cronInstance := startCronTask(cronLogger)
	if cronInstance == nil {
		fmt.Println("定时任务启动失败")
		return
	}
	defer cronInstance.Stop() // 程序退出前停止定时任务

	// 等待3秒确保任务执行完成
	time.Sleep(3 * time.Second)
	fmt.Println("=== 程序执行完成，请检查日志文件 ===")
}

// 数据库连接（必现错误）
func connectDB() error {
	return errors.New("数据库连接被拒绝（强制错误）")
}

// 启动定时任务（返回*cron.Cron实例，确保仅执行一次）
func startCronTask(cronLogger log.Logger) *cron.Cron {
	// 创建cron实例，使用自定义日志适配器（过滤内部日志）
	c := cron.New(
		cron.WithLogger(&cronLoggerAdapter{cronLogger}),
		cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger)), // 避免任务重叠
	)

	// 添加仅执行一次的任务（使用@every 0s确保立即执行且仅一次）
	taskSpec := "@every 0s"
	_, err := c.AddFunc(taskSpec, func() {
		taskLogger := cronLogger.WithValues("task", "data_backup")
		taskLogger.Info("开始执行备份任务（预期失败）") // info日志2
		if err := backupData(); err != nil {
			taskLogger.Error(err, "备份任务失败（业务错误）") // error日志3
		}
	})

	if err != nil {
		cronLogger.Error(err, "注册定时任务失败", "task_spec", taskSpec)
		return nil
	}

	// 启动定时任务
	c.Start()
	return c
}

// 备份任务（必现错误）
func backupData() error {
	return errors.New("备份目录空间不足（强制错误）")
}

// cron日志适配器（过滤内部调度日志，仅保留业务日志）
type cronLoggerAdapter struct {
	log.Logger
}

// Info 过滤cron内部日志（如start/wake/run），仅输出业务日志
func (c *cronLoggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	// 过滤包含cron内部关键词的日志
	internalKeywords := []string{"start", "wake", "run", "stop", "schedule"}
	for _, kw := range internalKeywords {
		if strings.Contains(msg, kw) {
			return // 跳过内部日志
		}
	}
	// 业务日志正常输出
	c.Logger.Info(msg, keysAndValues...)
}

// Error 错误日志正常输出
func (c *cronLoggerAdapter) Error(err error, msg string, keysAndValues ...interface{}) {
	c.Logger.Error(err, msg, keysAndValues...)
}

// Flush 日志刷新
func (c *cronLoggerAdapter) Flush() {
	log.Flush()
}
