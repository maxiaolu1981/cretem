// scripts/send_retry_test.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// 全局配置变量 - 在这里修改测试参数
var (
	// 测试用户配置
	TestUsername = "retry_test_user2"        // 修改用户名
	TestEmail    = "retry_test1@example.com" // 修改邮箱
	TestNickname = "重试测试用户"                  // 修改昵称
	TestPhone    = "13800000000"             // 修改手机号

	// Kafka配置
	KafkaBrokers = []string{"127.0.0.1:9092"} // 修改Kafka地址
	RetryTopic   = "user.retry.v1"            // 重试主题名称

	// 重试配置
	RetryCount = 1                // 当前重试次数
	RetryError = "模拟的数据库连接失败"     // 错误信息
	RetryDelay = 10 * time.Second // 重试延迟时间
)

func main() {
	fmt.Println("开始发送重试测试消息...")
	fmt.Printf("测试用户: %s\n", TestUsername)
	fmt.Printf("测试邮箱: %s\n", TestEmail)

	// 创建Kafka writer
	writer := &kafka.Writer{
		Addr:     kafka.TCP(KafkaBrokers...),
		Topic:    RetryTopic,
		Balancer: &kafka.LeastBytes{},
	}
	defer writer.Close()

	// 创建测试用户消息
	testUser := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      TestUsername,
			"createdAt": time.Now().Format(time.RFC3339),
			"updatedAt": time.Now().Format(time.RFC3339),
		},
		"status":    1,
		"nickname":  TestNickname,
		"password":  "$2a$10$encryptedpasswordforretrytest",
		"email":     TestEmail,
		"phone":     TestPhone,
		"loginedAt": time.Now().Format(time.RFC3339),
	}

	userData, err := json.Marshal(testUser)
	if err != nil {
		log.Fatal("JSON序列化失败: ", err)
	}

	ctx := context.Background()

	// 发送到重试主题
	err = writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(TestUsername),
		Value: userData,
		Headers: []kafka.Header{
			{Key: "operation", Value: []byte("create")},
			{Key: "original-timestamp", Value: []byte(time.Now().Add(-5 * time.Minute).Format(time.RFC3339))},
			{Key: "retry-count", Value: []byte(fmt.Sprintf("%d", RetryCount))},
			{Key: "retry-error", Value: []byte(RetryError)},
			{Key: "next-retry-ts", Value: []byte(time.Now().Add(RetryDelay).Format(time.RFC3339))},
		},
	})

	if err != nil {
		log.Fatal("发送重试消息失败: ", err)
	}

	fmt.Println("✅ 重试测试消息已发送到重试主题")
	fmt.Println("📋 消息详情:")
	fmt.Printf("  Key: %s\n", TestUsername)
	fmt.Printf("  Topic: %s\n", RetryTopic)
	fmt.Printf("  重试次数: %d\n", RetryCount)
	fmt.Printf("  下次重试时间: %s\n", time.Now().Add(RetryDelay).Format("15:04:05"))
	fmt.Printf("  错误信息: %s\n", RetryError)

	// 监控消息处理情况
	fmt.Println("\n🔍 开始监控消息处理情况...")

	// 检查消息是否成功写入
	checkMessageInTopic(TestUsername)

	// 等待重试消费者处理
	fmt.Printf("⏳ 等待重试消费者处理消息（%v）...\n", RetryDelay)
	time.Sleep(RetryDelay + 2*time.Second)

	// 检查消息是否被处理
	checkMessageProcessing()

	fmt.Println("\n📊 测试完成！")
}

func checkMessageInTopic(expectedKey string) {
	fmt.Printf("检查主题 %s 中是否存在消息...\n", RetryTopic)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   KafkaBrokers,
		Topic:     RetryTopic,
		Partition: 0,
		MaxWait:   3 * time.Second,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		fmt.Printf("❌ 无法读取主题 %s 的消息: %v\n", RetryTopic, err)
		return
	}

	if string(msg.Key) == expectedKey {
		fmt.Printf("✅ 找到测试消息: Key=%s, Offset=%d\n", string(msg.Key), msg.Offset)

		// 输出headers信息
		fmt.Println("📋 消息Headers:")
		for _, header := range msg.Headers {
			fmt.Printf("  %s: %s\n", header.Key, string(header.Value))
		}
	} else {
		fmt.Printf("⚠️  找到其他消息: Key=%s\n", string(msg.Key))
	}
}

func checkMessageProcessing() {
	fmt.Println("检查消息处理结果...")

	// 检查重试主题是否还有消息
	checkTopicStatus(RetryTopic, "重试主题")

	// 检查死信主题是否有消息（如果重试失败）
	checkTopicStatus("user.deadletter.v1", "死信主题")

	// 检查数据库是否创建了用户
	fmt.Printf("检查数据库是否创建了用户 '%s'...\n", TestUsername)
	fmt.Println("请手动检查数据库或查看应用程序日志")
}

func checkTopicStatus(topic, description string) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   KafkaBrokers,
		Topic:     topic,
		Partition: 0,
		MaxWait:   2 * time.Second,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := reader.ReadMessage(ctx)
	if err != nil {
		fmt.Printf("✅ %s: 无消息（可能已被处理）\n", description)
	} else {
		fmt.Printf("⚠️  %s: 仍有消息未处理\n", description)
	}
}
