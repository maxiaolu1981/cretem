// internal/pkg/server/retry_consumer_integration_test.go
package functional

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

// TestProcessRetryDelete_RealEnvironment 真实环境删除功能测试
func TestProcessRetryDelete_RealEnvironment(t *testing.T) {
	// 使用您真实的数据库连接
	db := getRealDBConnection()        // 替换为您的真实数据库连接获取方式
	redis := getRealRedisConnection()  // 替换为您的真实Redis连接
	producer := getRealKafkaProducer() // 替换为您的真实Kafka生产者

	kafkaOptions := &options.KafkaOptions{
		Brokers:        []string{"192.168.10.8:9092"}, // 使用您的真实Broker地址
		MaxRetries:     3,
		BaseRetryDelay: 1 * time.Second,
		MaxRetryDelay:  30 * time.Minute,
	}

	consumer := NewRetryConsumer(db, redis, producer, kafkaOptions, "user.retry.v1", "test-group")

	ctx := context.Background()

	// 测试用例1: 正常删除存在的用户
	t.Run("成功删除存在的用户", func(t *testing.T) {
		// 准备测试数据
		testUser := &v1.User{
			Name:     "test_user_delete_1",
			Email:    "test1@example.com",
			Password: "password123",
			Status:   1,
		}

		// 先创建用户
		if err := db.Create(testUser).Error; err != nil {
			t.Fatalf("创建测试用户失败: %v", err)
		}

		// 创建删除消息
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "test_user_delete_1",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		msg := kafka.Message{
			Key:   []byte("test_user_delete_1"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("0")},
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
			},
		}

		// 执行删除
		err := consumer.processRetryDelete(ctx, msg)
		if err != nil {
			t.Errorf("删除用户失败: %v", err)
		}

		// 验证用户已被删除
		var user v1.User
		result := db.Where("name = ?", "test_user_delete_1").First(&user)
		if result.Error == nil {
			t.Error("用户应该已被删除，但仍然存在")
		}
	})

	// 测试用例2: 删除不存在的用户（幂等性测试）
	t.Run("删除不存在的用户-幂等性", func(t *testing.T) {
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "non_existent_user_123",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		msg := kafka.Message{
			Key:   []byte("non_existent_user_123"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("0")},
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
			},
		}

		// 执行删除 - 应该成功（幂等性）
		err := consumer.processRetryDelete(ctx, msg)
		if err != nil {
			t.Errorf("删除不存在的用户应该成功（幂等性）: %v", err)
		}
	})

	// 测试用例3: DEFINER错误场景
	t.Run("DEFINER错误处理", func(t *testing.T) {
		// 这个测试需要确保触发器存在且DEFINER用户不存在
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "test_user_definer_error",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		msg := kafka.Message{
			Key:   []byte("test_user_definer_error"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("0")},
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
			},
		}

		// 执行删除
		err := consumer.processRetryDelete(ctx, msg)

		// 根据您的业务逻辑，DEFINER错误应该被忽略
		if err != nil {
			t.Logf("DEFINER错误（预期中）: %v", err)
			// 这里应该验证错误是否被正确分类为可忽略错误
		}
	})

	// 测试用例4: 达到最大重试次数
	t.Run("达到最大重试次数", func(t *testing.T) {
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "test_user_max_retries",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		msg := kafka.Message{
			Key:   []byte("test_user_max_retries"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("3")}, // 已达到最大重试次数
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
				{Key: HeaderRetryError, Value: []byte("模拟重复错误")},
			},
		}

		// 执行删除 - 应该进入死信队列
		err := consumer.processRetryDelete(ctx, msg)
		if err == nil {
			t.Error("达到最大重试次数应该返回错误")
		}

		// 验证是否调用了死信队列发送
		// 这里需要根据您的producer实现来验证
		t.Logf("最大重试次数测试完成: %v", err)
	})

	// 测试用例5: 重试时间未到
	t.Run("重试时间未到", func(t *testing.T) {
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "test_user_retry_time",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		futureTime := time.Now().Add(10 * time.Minute)
		msg := kafka.Message{
			Key:   []byte("test_user_retry_time"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("1")},
				{Key: HeaderNextRetryTS, Value: []byte(futureTime.Format(time.RFC3339))},
				{Key: HeaderRetryError, Value: []byte("previous error")},
			},
		}

		// 执行删除 - 应该重新入队
		err := consumer.processRetryDelete(ctx, msg)
		if err != nil {
			t.Logf("重试时间未到，重新入队: %v", err)
		}
	})

	// 测试用例6: 消息格式错误
	t.Run("消息格式错误", func(t *testing.T) {
		// 发送无效的JSON
		msg := kafka.Message{
			Key:   []byte("test_user_invalid_json"),
			Value: []byte("invalid json {"),
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("0")},
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
			},
		}

		// 执行删除 - 应该进入死信队列
		err := consumer.processRetryDelete(ctx, msg)
		if err == nil {
			t.Error("无效JSON消息应该返回错误")
		}
	})

	// 测试用例7: 数据库连接错误
	t.Run("数据库连接错误", func(t *testing.T) {
		// 这个测试需要模拟数据库连接失败
		// 可以通过临时关闭数据库连接或使用连接字符串错误来实现
		deleteReq := struct {
			Username  string `json:"username"`
			DeletedAt string `json:"deleted_at"`
		}{
			Username:  "test_user_db_error",
			DeletedAt: time.Now().Format(time.RFC3339),
		}

		value, _ := json.Marshal(deleteReq)
		msg := kafka.Message{
			Key:   []byte("test_user_db_error"),
			Value: value,
			Headers: []kafka.Header{
				{Key: HeaderOperation, Value: []byte(OperationDelete)},
				{Key: HeaderRetryCount, Value: []byte("0")},
				{Key: HeaderNextRetryTS, Value: []byte(time.Now().Add(-time.Hour).Format(time.RFC3339))},
			},
		}

		// 临时修改数据库连接为无效连接
		originalDB := consumer.db
		// 这里需要根据您的DB配置创建无效连接
		// consumer.db = createInvalidDBConnection()

		err := consumer.processRetryDelete(ctx, msg)

		// 恢复数据库连接
		consumer.db = originalDB

		if err != nil {
			t.Logf("数据库连接错误（预期中）: %v", err)
		}
	})
}

// 辅助函数 - 需要根据您的实际环境实现
func getRealDBConnection() *gorm.DB {
	// 返回您真实的数据库连接
	// 例如: return database.GetDB()
	return nil
}

func getRealRedisConnection() *storage.RedisCluster {
	// 返回您真实的Redis连接
	// 例如: return redis.GetCluster()
	return nil
}

func getRealKafkaProducer() *UserProducer {
	// 返回您真实的Kafka生产者
	// 例如: return NewUserProducer(kafkaOptions)
	return nil
}

// TestIsIgnorableDeleteError 测试错误分类逻辑
func TestIsIgnorableDeleteError(t *testing.T) {
	consumer := &RetryConsumer{}

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"DEFINER错误", fmt.Errorf("definer iam@127.0.0.1 does not exist"), true},
		{"数据不存在错误", fmt.Errorf("record not found"), true},
		{"用户不存在", fmt.Errorf("does not exist"), true},
		{"重复键错误", fmt.Errorf("duplicate entry"), true},
		{"MySQL重复错误码", fmt.Errorf("Error 1062"), true},
		{"普通错误", fmt.Errorf("some other error"), false},
		{"权限错误", fmt.Errorf("permission denied"), false},
		{"语法错误", fmt.Errorf("syntax error"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := consumer.isIgnorableDeleteError(tc.err)
			if result != tc.expected {
				t.Errorf("isIgnorableDeleteError(%q) = %v, 期望 %v", tc.err.Error(), result, tc.expected)
			}
		})
	}
}

// TestShouldIgnoreError 测试全局错误忽略逻辑
func TestShouldIgnoreError(t *testing.T) {
	testCases := []struct {
		name      string
		errorInfo string
		expected  bool
	}{
		{"DEFINER错误", "definer iam@127.0.0.1 does not exist", true},
		{"数据不存在", "record not found", true},
		{"用户已存在", "user already exist", true},
		{"MySQL重复错误", "Error 1062", true},
		{"无效JSON", "invalid json", false},    // 应该进入死信队列
		{"未知操作", "unknown operation", false}, // 应该进入死信队列
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldIgnoreError(tc.errorInfo)
			if result != tc.expected {
				t.Errorf("shouldIgnoreError(%q) = %v, 期望 %v", tc.errorInfo, result, tc.expected)
			}
		})
	}
}
