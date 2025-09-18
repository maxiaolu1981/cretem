// scripts/test_message_flow.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
    brokers := []string{"127.0.0.1:9092"}
    
    // 1. 发送测试消息到 create 主题
    testMsg := map[string]interface{}{
        "metadata": map[string]interface{}{
            "name":      "test_user_123",
            "createdAt": time.Now().Format(time.RFC3339),
        },
        "nickname": "测试用户",
        "password": "$2a$10$test",
        "email":    "test@example.com",
    }
    
    msgData, _ := json.Marshal(testMsg)
    
    writer := &kafka.Writer{
        Addr:  kafka.TCP(brokers...),
        Topic: "user.create.v1",
    }
    
    err := writer.WriteMessages(context.Background(), kafka.Message{
        Key:   []byte("test_user_123"),
        Value: msgData,
        Headers: []kafka.Header{
            {Key: "operation", Value: []byte("create")},
            {Key: "test-marker", Value: []byte("flow-test")},
        },
    })
    
    if err != nil {
        log.Fatal("发送测试消息失败:", err)
    }
    fmt.Println("✅ 测试消息已发送到 user.create.v1")
    
    // 2. 监控消息流向
    monitorMessageFlow("test_user_123")
}

func monitorMessageFlow(key string) {
    fmt.Printf("🔍 开始监控消息流向: key=%s\n", key)
    
    topics := []string{"user.create.v1", "user.retry.v1", "user.deadletter.v1"}
    
    for _, topic := range topics {
        fmt.Printf("检查主题 %s...\n", topic)
        reader := kafka.NewReader(kafka.ReaderConfig{
            Brokers:   []string{"127.0.0.1:9092"},
            Topic:     topic,
            Partition: 0,
            MaxWait:   2 * time.Second,
        })
        defer reader.Close()
        
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        
        msg, err := reader.ReadMessage(ctx)
        if err != nil {
            fmt.Printf("  %s: 无消息或超时\n", topic)
            continue
        }
        
        if string(msg.Key) == key {
            fmt.Printf("  ✅ 在 %s 找到测试消息\n", topic)
            fmt.Printf("     Headers: %+v\n", msg.Headers)
        }
    }
}