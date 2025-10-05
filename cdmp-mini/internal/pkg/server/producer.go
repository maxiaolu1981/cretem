// internal/pkg/server/producer.go
package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/segmentio/kafka-go"
)

var _ producer.MessageProducer = (*UserProducer)(nil)
var KafkaBrokers = []string{"192.168.10.8:9092", "192.168.10.8:9093", "192.168.10.8:9094"}

type UserProducer struct {
	writer       *kafka.Writer
	retryWriter  *kafka.Writer
	kafkaOptions *options.KafkaOptions
	maxRetries   int
}

// internal/pkg/server/producer.go

func NewUserProducer(options *options.KafkaOptions) *UserProducer {
	// 主Writer（高性能配置）主业务消息（创建、更新、删除用户）
	mainWriter := &kafka.Writer{
		Addr: kafka.TCP(options.Brokers...),
		// 注意：这里不设置 Topic，在发送时动态设置
		MaxAttempts:     3,
		WriteBackoffMin: 100 * time.Millisecond,
		WriteBackoffMax: 1 * time.Second,
		BatchBytes:      1048576,
		BatchSize:       options.BatchSize,
		BatchTimeout:    options.BatchTimeout,
		WriteTimeout:    30 * time.Second,
		Balancer:        &kafka.LeastBytes{},
		Compression:     kafka.Snappy,
		RequiredAcks:    kafka.RequireOne,
		Async:           false,
	}

	// 重试Writer（高可靠配置）- 不设置 Topic
	reliableWriter := &kafka.Writer{
		Addr: kafka.TCP(options.Brokers...),
		// 注意：这里不设置 Topic，在发送时动态设置
		//	Topic:           UserRetryTopic, // ✅ 在这里设置Topic
		MaxAttempts:     10, // 更多重试次数
		WriteBackoffMin: 500 * time.Millisecond,
		WriteBackoffMax: 5 * time.Second,
		BatchSize:       1,                // 每条消息立即发送
		WriteTimeout:    60 * time.Second, // 更长超时
		RequiredAcks:    kafka.RequireAll, // 需要所有副本确认
		Async:           false,
		Compression:     kafka.Snappy,
		Balancer:        &kafka.Hash{}, // 新增：使用Hash平衡器确保分区均匀分布
	}

	return &UserProducer{
		writer:       mainWriter,
		retryWriter:  reliableWriter,
		maxRetries:   options.MaxRetries,
		kafkaOptions: options,
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
	start := time.Now()
	var errSend error
	defer func() {
		metrics.RecordKafkaProducerOperation(topic, operation, time.Since(start).Seconds(), errSend, false)
	}()

	userData, err := json.Marshal(user)
	if err != nil {
		errSend = err
		log.Errorf("topic:%v operation:%v消息序列号失败:%v", topic, operation, err)
		return errors.WithCode(code.ErrEncodingJSON, "用户消息序列化失败")
	}
	now := time.Now()
	idBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(idBytes, user.ID)
	msg := kafka.Message{
		Key:   idBytes,
		Value: userData,
		Time:  now,
		Headers: []kafka.Header{
			{Key: HeaderOperation, Value: []byte(operation)},
			{Key: HeaderOriginalTimestamp, Value: []byte(now.Format(time.RFC3339))},
			{Key: HeaderRetryCount, Value: []byte("0")},
		},
	}
	errSend = p.sendWithRetry(ctx, msg, topic)
	return errSend
}

// 添加验证方法
func (p *UserProducer) validateMessage(msg kafka.Message) error {
	// 检查消息是否包含Topic字段（不应该包含）
	if strings.TrimSpace(msg.Topic) != "" {
		err := errors.WithCode(code.ErrMissingHeader, "必须设置topic")
		log.Errorf("%v %v", errors.GetMessage(err), err)
		return err
	}
	return nil
}

// sendWithRetry 带重试的发送逻辑
func (p *UserProducer) sendWithRetry(ctx context.Context, msg kafka.Message, topic string) error {
	startTime := time.Now()
	// 添加详细的发送日志
	//log.Errorf("准备发送消息到[测试丢失记录问题] %s: key=%s", topic, string(msg.Key))
	operation := p.getOperationFromHeaders(msg.Headers)

	var sendErr error
	var isRetry bool
	var success bool

	defer func() {
		// 只有成功或最终失败时才记录指标
		if success || sendErr != nil {
			metrics.RecordKafkaProducerOperation(topic, operation,
				time.Since(startTime).Seconds(), sendErr, isRetry)
		}
	}()

	if err := p.validateMessage(msg); err != nil {
		sendErr = err
		return err
	}

	// 创建新的消息
	sendMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Time:    time.Now(),
		Topic:   topic,
		Headers: make([]kafka.Header, len(msg.Headers)),
	}
	copy(sendMsg.Headers, msg.Headers)

	//首次发送
	err := p.writer.WriteMessages(ctx, sendMsg)
	if err == nil {
		success = true // 标记为成功
		log.Debugf("发送成功: topic=%s, key=%s, 耗时=%v", topic, string(msg.Key), time.Since(startTime))
		return nil
	}
	// 首次发送失败，进行重试
	isRetry = true
	sendErr = p.sendToRetryTopic(ctx, msg, err.Error())

	if sendErr != nil {
		return sendErr
	}

	success = true
	sendErr = nil
	log.Debugf("发送成功(重试): topic=%s, key=%s, 耗时=%v", topic, string(msg.Key), time.Since(startTime))
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

	// 空指针保护
	if p == nil {
		log.Error("UserProducer is nil in sendToRetryTopic")
		return fmt.Errorf("user producer is nil")
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

	//检查最大重试次数
	if currentRetryCount > p.maxRetries {
		errMsg := fmt.Sprintf("已达最大重试次数（%d次）,将发送到死信区", p.maxRetries)
		return p.SendToDeadLetterTopic(ctx, msg, errMsg+": "+errorInfo)
	}

	// 3. 创建重试消息
	retryMsg := kafka.Message{
		Key:   msg.Key,
		Value: msg.Value,
		Time:  time.Now(),
		//	Topic: UserRetryTopic,
	}

	// 复制并更新headers
	retryMsg.Headers = make([]kafka.Header, len(msg.Headers))
	copy(retryMsg.Headers, msg.Headers)
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderRetryCount, strconv.Itoa(currentRetryCount))
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderRetryError, errorInfo)
	retryMsg.Headers = p.updateOrAddHeader(retryMsg.Headers, HeaderNextRetryTS, p.calcNextRetryTS(currentRetryCount).Format(time.RFC3339))

	// 4. 使用增强的同步发送（不改变原有函数名）
	retryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := p.sendMessageWithRetry(retryCtx, retryMsg, UserRetryTopic)
	if err != nil {
		log.Errorf("重试发送失败（key=%s, 第%d次）: %v", string(msg.Key), currentRetryCount, err)
		// 进入死信队列
		return p.SendToDeadLetterTopic(ctx, msg, "重试发送失败: "+err.Error())
	}
	return nil
}

// 新增：计算下次重试时间（指数退避策略）
func (p *UserProducer) calcNextRetryTS(retryCount int) time.Time {

	if retryCount <= 0 {
		retryCount = 1 // 或者返回默认延迟
	}
	// 基础延迟 * 2^(重试次数-1)，避免短期内频繁重试
	delay := p.kafkaOptions.BaseRetryDelay * time.Duration(1<<(retryCount-1))
	// 限制最大延迟，避免重试间隔过长
	if delay > p.kafkaOptions.MaxRetryDelay {
		delay = p.kafkaOptions.MaxRetryDelay
	}
	return time.Now().Add(delay)
}

func (p *UserProducer) SendToDeadLetterTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	start := time.Now()
	operation := p.getOperationFromHeaders(msg.Headers)

	var sendErr error
	defer func() {
		p.recordDeadLetterOperation(UserDeadLetterTopic, operation, start, sendErr, errorInfo, string(msg.Key))
	}()
	// 创建死信消息
	deadLetterMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Time:    time.Now(),
		Headers: p.updateOrAddHeader(msg.Headers, "deadletter-reason", errorInfo),
	}
	deadLetterMsg.Headers = p.updateOrAddHeader(deadLetterMsg.Headers, "deadletter-timestamp", time.Now().Format(time.RFC3339))

	// 使用增强的同步发送
	sendErr = p.sendMessageWithRetry(ctx, deadLetterMsg, UserDeadLetterTopic)
	return sendErr
}

// recordDeadLetterOperation 记录死信队列操作指标
func (p *UserProducer) recordDeadLetterOperation(topic, operation string, start time.Time, err error, errorInfo, messageKey string) {
	duration := time.Since(start).Seconds()

	// 记录死信消息计数
	metrics.DeadLetterMessages.WithLabelValues(topic, operation).Inc()

	// 记录处理时间
	status := "dead_letter_success"
	if err != nil {
		status = "dead_letter_failure"
		// 记录死信发送失败的错误
		errorType := metrics.GetKafkaErrorType(err)
		metrics.ProducerFailures.WithLabelValues(topic, operation, errorType).Inc()
	}
	metrics.MessageProcessingTime.WithLabelValues(topic, operation, status).Observe(duration)

	// 记录日志
	if err != nil {
		log.Errorf("死信队列发送失败: key=%s, reason=%s, error=%v", messageKey, errorInfo, err)
	} else {
		log.Debugf("发送到死信队列成功: key=%s, reason=%s, 耗时=%v", messageKey, errorInfo, duration)
	}
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

// sendMessageWithRetry 增强的同步发送方法
func (p *UserProducer) sendMessageWithRetry(ctx context.Context, msg kafka.Message, topic string) error {
	const maxSendRetries = 3
	var lastErr error

	for i := 0; i < maxSendRetries; i++ {
		// 使用临时writer，避免长期占用连接,不设置Topic
		writer := &kafka.Writer{
			Addr:                   kafka.TCP(KafkaBrokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{},
			BatchSize:              1, // 同步发送，每批次1条
			BatchTimeout:           100 * time.Millisecond,
			Async:                  false,            // 同步模式
			RequiredAcks:           kafka.RequireOne, // 只需要leader确认
			AllowAutoTopicCreation: true,             // 允许自动创建主题
		}

		// 确保消息有Topic
		sendMsg := kafka.Message{
			Key:     msg.Key, // ✅ 有Key才能均匀分区
			Value:   msg.Value,
			Time:    msg.Time,
			Headers: msg.Headers,
			Topic:   topic, // ✅ 确保设置topic
		}

		// 设置发送超时
		sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := writer.WriteMessages(sendCtx, sendMsg)

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

		if i < maxSendRetries-1 {
			baseDelay := time.Second
			maxDelay := 30 * time.Second
			// 指数退避：1s, 2s, 4s, 8s, 16s, 30s, 30s...
			waitTime := baseDelay * time.Duration(1<<uint(i))
			if waitTime > maxDelay {
				waitTime = maxDelay
			}

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
