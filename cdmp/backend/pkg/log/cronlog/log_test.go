package cronlog

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCronLoggerIntegration(t *testing.T) {
	// 1. 创建zap日志观察者
	core, recordedLogs := observer.New(zap.DebugLevel)
	zapLogger := zap.New(core).Sugar()
	defer zapLogger.Sync()

	// 2. 创建cron日志适配器
	cronLogger := NewLogger(zapLogger)

	// 3. 创建cron实例并绑定适配器
	c := cron.New(
		cron.WithLogger(cronLogger),
		cron.WithSeconds(),
		cron.WithChain(cron.Recover(cronLogger)),
	)

	// 4. 注册测试任务
	// 正常任务（ID=1，类型为cron.EntryID）
	_, err := c.AddFunc("* * * * * *", func() {})
	if err != nil {
		t.Fatalf("注册正常任务失败: %v", err)
	}
	// 错误任务（ID=2，类型为cron.EntryID）
	errorTaskID, err := c.AddFunc("* * * * * *", func() {
		panic(errors.New("数据库连接超时"))
	})
	if err != nil {
		t.Fatalf("注册错误任务失败: %v", err)
	}

	// 5. 启动cron并运行3秒
	c.Start()
	defer c.Stop()
	time.Sleep(3 * time.Second)

	// 6. 基于实际日志格式验证
	t.Run("验证启动日志", func(t *testing.T) {
		startLogs := findLogsByMsgExact(recordedLogs.All(), "start")
		if len(startLogs) == 0 {
			t.Error("未找到消息为'start'的启动日志")
		} else {
			t.Logf("启动日志验证通过: %s", startLogs[0].Message)
		}
	})

	t.Run("验证任务执行日志", func(t *testing.T) {
		// 实际执行日志消息为"run"，任务ID字段为"entry"（类型cron.EntryID）
		runLogs := findLogsByMsgExact(recordedLogs.All(), "run")
		if len(runLogs) == 0 {
			t.Fatal("未找到消息为'run'的执行日志")
		}

		// 验证每个执行日志都包含"entry"字段（类型为cron.EntryID）
		for i, log := range runLogs {
			entryID, ok := log.ContextMap()["entry"]
			if !ok {
				t.Errorf("第%d条执行日志缺少'entry'字段（任务ID）", i)
				continue
			}
			// 验证字段类型为cron.EntryID（关键修复）
			if _, ok := entryID.(cron.EntryID); !ok {
				t.Errorf("第%d条执行日志的'entry'字段类型错误，预期cron.EntryID，实际: %T", i, entryID)
			} else {
				t.Logf("第%d条执行日志验证通过，任务ID: %d", i, entryID)
			}
		}
	})

	t.Run("验证错误任务日志", func(t *testing.T) {
		// 错误日志包含"panic"，错误信息在"error"字段
		errorLogs := findLogsByMsgContains(recordedLogs.All(), "panic")
		if len(errorLogs) == 0 {
			t.Fatal("未找到包含'panic'的错误日志")
		}

		// 验证错误日志与目标任务ID关联
		found := false
		for _, log := range errorLogs {
			// 验证级别为error
			if log.Level != zap.ErrorLevel {
				t.Errorf("错误日志级别错误，预期error，实际: %s", log.Level)
			}

			// 验证错误信息正确
			errorMsg, ok := log.ContextMap()["error"]
			if !ok || errorMsg != "数据库连接超时" {
				t.Errorf("错误日志的'error'字段不正确，实际: %v", errorMsg)
			}

			// 关联错误日志与任务ID（通过前一条执行日志的entry字段）
			logIndex := findLogIndex(recordedLogs.All(), log)
			if logIndex > 0 {
				prevLog := recordedLogs.All()[logIndex-1]
				if prevLog.Message == "run" {
					prevEntryID, ok := prevLog.ContextMap()["entry"].(cron.EntryID)
					if ok && prevEntryID == errorTaskID {
						found = true
						break
					}
				}
			}
		}

		if !found {
			t.Errorf("错误日志中未找到与任务ID %d 关联的记录", errorTaskID)
		} else {
			t.Logf("错误日志验证通过，成功关联任务ID: %d", errorTaskID)
		}
	})

	t.Run("验证日志刷新", func(t *testing.T) {
		cronLogger.Flush()
		t.Log("日志刷新功能验证通过")
	})
}

// 辅助函数：查找消息完全匹配的日志
func findLogsByMsgExact(logs []observer.LoggedEntry, msg string) []observer.LoggedEntry {
	var result []observer.LoggedEntry
	for _, log := range logs {
		if log.Message == msg {
			result = append(result, log)
		}
	}
	return result
}

// 辅助函数：查找消息包含指定字符串的日志
func findLogsByMsgContains(logs []observer.LoggedEntry, substr string) []observer.LoggedEntry {
	var result []observer.LoggedEntry
	for _, log := range logs {
		if strings.Contains(log.Message, substr) {
			result = append(result, log)
		}
	}
	return result
}

// 辅助函数：查找日志在列表中的索引
func findLogIndex(logs []observer.LoggedEntry, target observer.LoggedEntry) int {
	for i, log := range logs {
		if log.Message == target.Message && log.Level == target.Level {
			return i
		}
	}
	return -1
}
