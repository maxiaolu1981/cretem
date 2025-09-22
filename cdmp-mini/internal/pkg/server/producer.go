// internal/pkg/server/producer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/segmentio/kafka-go"
)

var _ producer.MessageProducer = (*UserProducer)(nil)
var KafkaBrokers = []string{"127.0.0.1:9092"}

// 在全局变量区域添加 Prometheus 指标

type UserProducer struct {
	writer           *kafka.Writer
	retryWriter      *kafka.Writer
	maxRetries       int
	deadLetterWriter *kafka.Writer
}

// internal/pkg/server/producer.go

func NewUserProducer(brokers []string, batchSize int, batchTimeout time.Duration) *UserProducer {
	// 主Writer（高性能配置）
	mainWriter := &kafka.Writer{
		Addr: kafka.TCP(brokers...),
		// 注意：这里不设置 Topic，在发送时动态设置
		MaxAttempts:     3,
		WriteBackoffMin: 100 * time.Millisecond,
		WriteBackoffMax: 1 * time.Second,
		BatchBytes:      1048576,
		BatchSize:       batchSize,
		BatchTimeout:    batchTimeout,
		WriteTimeout:    30 * time.Second,
		Balancer:        &kafka.LeastBytes{},
		Compression:     kafka.Snappy,
		RequiredAcks:    kafka.RequireOne,
		Async:           false,
	}

	// 重试Writer（高可靠配置）- 不设置 Topic
	reliableWriter := &kafka.Writer{
		Addr: kafka.TCP(brokers...),
		// 注意：这里不设置 Topic，在发送时动态设置
		MaxAttempts:     10,
		WriteBackoffMin: 500 * time.Millisecond,
		WriteBackoffMax: 5 * time.Second,
		BatchSize:       1,
		WriteTimeout:    60 * time.Second,
		RequiredAcks:    kafka.RequireAll,
		Async:           false,
		Compression:     kafka.Snappy,
	}

	return &UserProducer{
		writer:      mainWriter,
		retryWriter: reliableWriter,
		maxRetries:  MaxRetryCount,
	}
}

func (p *UserProducer) SendUserCreateMessage(ctx context.Context, user *v1.User) error {
	return p.sendUserMessage(ctx, user, OperationCreate, UserCreateTopic)
}

func (p *UserProducer) SendUserUpdateMessage(ctx context.Context, user *v1.User) error {
	return p.sendUserMessage(ctx, user, OperationUpdate, UserUpdateTopic)
}

func (p *UserProducer) SendUserDeleteMessage(ctx context.Context, username string) error {
	deleteData := map[string]interface{}{
		"username":   username,
		"deleted_at": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(deleteData)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "删除消息序列化失败")
	}

	msg := kafka.Message{
		Key:   []byte(username),
		Value: data,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: HeaderOperation, Value: []byte(OperationDelete)},
			{Key: HeaderOriginalTimestamp, Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: HeaderRetryCount, Value: []byte("0")},
		},
	}

	return p.sendWithRetry(ctx, msg, UserDeleteTopic)
}

func (p *UserProducer) sendUserMessage(ctx context.Context, user *v1.User, operation, topic string) error {
	userData, err := json.Marshal(user)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "用户消息序列化失败")
	}

	msg := kafka.Message{
		Key:   []byte(user.Name),
		Value: userData,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: HeaderOperation, Value: []byte(operation)},
			{Key: HeaderOriginalTimestamp, Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: HeaderRetryCount, Value: []byte("0")},
		},
	}

	return p.sendWithRetry(ctx, msg, topic)
}

// 添加验证方法
func (p *UserProducer) validateMessage(msg kafka.Message) error {
	// 检查消息是否包含Topic字段（不应该包含）
	if msg.Topic != "" {
		return fmt.Errorf("message should not have Topic field set")
	}
	return nil
}

// sendWithRetry 带重试的发送逻辑
func (p *UserProducer) sendWithRetry(ctx context.Context, msg kafka.Message, topic string) error {
	startTime := time.Now()
	operation := p.getOperationFromHeaders(msg.Headers)

	// 记录发送尝试
	metrics.ProducerAttempts.WithLabelValues(topic, operation).Inc()

	// 创建新的消息
	sendMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Time:    time.Now(),
		Topic:   topic,
		Headers: make([]kafka.Header, len(msg.Headers)),
	}
	copy(sendMsg.Headers, msg.Headers)

	if err := p.validateMessage(msg); err != nil {
		return fmt.Errorf("invalid message: %v", err)
	}

	err := p.writer.WriteMessages(ctx, sendMsg)

	if err != nil {
		log.Errorf("Topic %s 发送失败，尝试重试Topic. Key: %s", topic, string(msg.Key))
		// 记录失败
		metrics.ProducerFailures.WithLabelValues(topic, operation, "initial_send").Inc()

		retryErr := p.sendToRetryTopic(ctx, msg, err.Error())
		if retryErr != nil {
			// 记录重试失败的总时间
			metrics.MessageProcessingTime.WithLabelValues(topic, operation, "failure").Observe(time.Since(startTime).Seconds())
			return retryErr
		}
		// 记录重试成功
		metrics.ProducerRetries.WithLabelValues(topic, operation).Inc()
		metrics.ProducerSuccess.WithLabelValues(topic, operation).Inc() // ✅ 重试成功也要记录成功！
		metrics.MessageProcessingTime.WithLabelValues(topic, operation, "retry_success").Observe(time.Since(startTime).Seconds())
		return nil
	}

	// ✅ 修复：直接成功时记录成功指标
	metrics.ProducerSuccess.WithLabelValues(topic, operation).Inc()
	metrics.MessageProcessingTime.WithLabelValues(topic, operation, "success").Observe(time.Since(startTime).Seconds())
	log.Infow("消息成功发送到Topic", "topic", topic, "key", string(msg.Key))
	return nil
}

func (p *UserProducer) getOperationFromHeaders(headers []kafka.Header) string {
	for _, h := range headers {
		if h.Key == HeaderOperation {
			return string(h.Value)
		}
	}
	return "unknown"
}

func (p *UserProducer) sendToRetryTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	// 1. 读取原始消息的重试次数
	log.Debugf("📨 进入sendToRetryTopic: key=%s", string(msg.Key))

	for i, header := range msg.Headers {
		log.Debugf("  输入消息Header[%d]: %s=%s", i, header.Key, string(header.Value))
	}
	currentRetryCount := 0
	for _, h := range msg.Headers {
		if h.Key == HeaderRetryCount {
			if cnt, err := strconv.Atoi(string(h.Value)); err == nil {
				currentRetryCount = cnt
			}
			break
		}
	}
	currentRetryCount++

	// 2. 检查最大重试次数
	if currentRetryCount > p.maxRetries {
		errMsg := fmt.Sprintf("已达最大重试次数（%d次）", p.maxRetries)
		log.Warnf("key=%s: %s", string(msg.Key), errMsg)
		return p.SendToDeadLetterTopic(ctx, msg, errMsg+": "+errorInfo)
	}

	// 3. 创建重试消息
	retryMsg := kafka.Message{
		Key:   msg.Key,
		Value: msg.Value,
		Time:  time.Now(),
	}

	// 复制并更新headers（修复重复问题）
	retryMsg.Headers = make([]kafka.Header, len(msg.Headers))
	copy(retryMsg.Headers, msg.Headers)
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderRetryCount, strconv.Itoa(currentRetryCount))
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderRetryError, errorInfo)
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderNextRetryTS, p.calcNextRetryTS(currentRetryCount).Format(time.RFC3339))

	// 4. 使用增强的同步发送（不改变原有函数名）
	retryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := p.sendMessageWithRetry(retryCtx, retryMsg, UserRetryTopic, currentRetryCount)
	if err != nil {
		log.Errorf("重试发送失败（key=%s, 第%d次）: %v", string(msg.Key), currentRetryCount, err)
		// 进入死信队列
		return p.SendToDeadLetterTopic(ctx, msg, "重试发送失败: "+err.Error())
	}

	log.Infow("成功发送到重试Topic",
		"key", string(msg.Key),
		"retry_count", currentRetryCount,
		"next_retry", p.calcNextRetryTS(currentRetryCount).Format(time.RFC3339))
	return nil
}

// 新增：计算下次重试时间（指数退避策略）
func (p *UserProducer) calcNextRetryTS(retryCount int) time.Time {
	// 基础延迟 * 2^(重试次数-1)，避免短期内频繁重试
	delay := BaseRetryDelay * time.Duration(1<<(retryCount-1))
	// 限制最大延迟，避免重试间隔过长
	if delay > MaxRetryDelay {
		delay = MaxRetryDelay
	}
	return time.Now().Add(delay)
}



func (p *UserProducer) SendToDeadLetterTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	operation := p.getOperationFromHeaders(msg.Headers)
	metrics.DeadLetterMessages.WithLabelValues(UserDeadLetterTopic, operation).Inc()
	// 创建死信消息
	deadLetterMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Time:    time.Now(),
		Headers: p.updateOrAddHeader(msg.Headers, "deadletter-reason", errorInfo),
	}
	deadLetterMsg.Headers = p.updateOrAddHeader(deadLetterMsg.Headers, "deadletter-timestamp", time.Now().Format(time.RFC3339))

	log.Warnf("发送到死信队列: key=%s, reason=%s", string(msg.Key), errorInfo)

	// 使用增强的同步发送
	return p.sendMessageWithRetry(ctx, deadLetterMsg, UserDeadLetterTopic, 0)
}

func (p *UserProducer) Close() error {
	if p.writer != nil {
		if err := p.writer.Close(); err != nil {
			return err
		}
	}

	if p.retryWriter != nil {
		if err := p.retryWriter.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (p *UserProducer) updateOrAddHeader(headers []kafka.Header, key, value string) []kafka.Header {
	// 1. 基础校验：阻断空Key输入（避免无效头）
	if key == "" {
		panic("kafka header key cannot be empty string")
	}

	// 2. 统一目标Key为小写（用于忽略大小写匹配，不改变原始Key的展示）
	targetKeyLower := strings.ToLower(key)

	// 3. 初始化新切片+标记：newHeaders存最终结果，found标记是否已保留一个目标头
	var newHeaders []kafka.Header
	foundTargetHeader := false

	// 4. 遍历原始切片：逐个处理每个头，筛选重复目标头
	for _, header := range headers {
		// 4.1 判断当前头是否为目标头（忽略大小写）
		currentHeaderKeyLower := strings.ToLower(header.Key)
		if currentHeaderKeyLower == targetKeyLower {
			// 4.2 若未保留过目标头：更新其Value，加入新切片，标记已保留
			if !foundTargetHeader {
				// 保留原始Key的大小写（仅更新Value），避免修改用户输入的Key格式
				updatedHeader := kafka.Header{
					Key:   header.Key,    // 如原Key是"Retry-Error"，仍保留该格式
					Value: []byte(value), // 写入最新Value
				}
				newHeaders = append(newHeaders, updatedHeader)
				foundTargetHeader = true // 标记已保留，后续重复头不再处理
			}
			// 4.3 若已保留过目标头：直接跳过，不加入新切片（删除重复）
			continue
		}

		// 4.4 非目标头：直接加入新切片（保持原有逻辑不变）
		newHeaders = append(newHeaders, header)
	}

	// 5. 若遍历完未找到任何目标头：新增一个（保留用户输入的原始Key大小写）
	if !foundTargetHeader {
		newHeaders = append(newHeaders, kafka.Header{
			Key:   key, // 如用户传"Retry-Error"，新增时就用该Key，不强制小写
			Value: []byte(value),
		})
	}

	return newHeaders
}

// sendMessageWithRetry 增强的同步发送方法（新增）
// sendMessageWithRetry 增强的同步发送方法
func (p *UserProducer) sendMessageWithRetry(ctx context.Context, msg kafka.Message, topic string, attempt int) error {
	const maxSendRetries = 3
	var lastErr error

	for i := 0; i < maxSendRetries; i++ {
		// 使用临时writer，避免长期占用连接
		writer := &kafka.Writer{
			Addr:                   kafka.TCP(KafkaBrokers...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			BatchSize:              1, // 同步发送，每批次1条
			BatchTimeout:           100 * time.Millisecond,
			Async:                  false,            // 同步模式
			RequiredAcks:           kafka.RequireOne, // 只需要leader确认
			AllowAutoTopicCreation: true,             // 允许自动创建主题
		}

		// 设置发送超时
		sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := writer.WriteMessages(sendCtx, msg)

		// 无论成功失败都立即关闭writer
		if closeErr := writer.Close(); closeErr != nil {
			log.Warnf("关闭writer失败: %v", closeErr)
		}
		if err == nil {
			return nil
		}

		lastErr = err
		log.Warnf("发送失败（key=%s, topic=%s, 尝试%d/%d）: %v",
			string(msg.Key), topic, i+1, maxSendRetries, err)

		// 等待后重试（指数退避）
		if i < maxSendRetries-1 {
			waitTime := time.Duration(1<<uint(i)) * time.Second
			select {
			case <-time.After(waitTime):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("发送到主题%s重试%d次均失败: %v", topic, maxSendRetries, lastErr)
}
