package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/ratelimiter"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/segmentio/kafka-go"
)

var _ producer.MessageProducer = (*UserProducer)(nil)

type UserProducer struct {
	producer     sarama.AsyncProducer
	kafkaOptions *options.KafkaOptions
	wg           sync.WaitGroup
	shutdown     chan struct{}
	limiter      *ratelimiter.RateLimiterController
	fallbackDir  string // 新增：降级文件目录
}

func NewUserProducer(
	options *options.KafkaOptions,
	limiter *ratelimiter.RateLimiterController,
	fallbackDir string,
) (*UserProducer, error) {
	log.Infof("[Producer] Initializing with options: %+v", options)

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal       // 只需要leader确认
	config.Producer.Compression = sarama.CompressionSnappy   // 压缩
	config.Producer.Flush.Frequency = options.FlushFrequency // 刷新频率
	config.Producer.Flush.MaxMessages = options.FlushMaxMessages
	config.Producer.Return.Successes = true // 异步发送成功后,返回成功消息
	config.Producer.Return.Errors = true    // 异步发送失败后,返回失败消息

	producer, err := sarama.NewAsyncProducer(options.Brokers, config)
	if err != nil {
		log.Errorf("Failed to create Sarama async producer: %v", err)
		return nil, fmt.Errorf("failed to create async producer: %w", err)
	}

	up := &UserProducer{
		producer:     producer,
		kafkaOptions: options,
		shutdown:     make(chan struct{}),
		limiter:      limiter,
		fallbackDir:  fallbackDir, // 保存降级目录
	}

	up.wg.Add(2)
	go up.handleSuccesses()
	go up.handleErrors()

	return up, nil
}

func (p *UserProducer) handleSuccesses() {
	defer p.wg.Done()
	for {
		select {
		case success := <-p.producer.Successes():
			if success != nil {
				log.Debugf("Message sent successfully to topic %s, partition %d, offset %d", success.Topic, success.Partition, success.Offset)
			}
		case <-p.shutdown:
			return
		}
	}
}

func (p *UserProducer) handleErrors() {
	defer p.wg.Done()
	for {
		select {
		case err := <-p.producer.Errors():
			if err != nil {
				log.Errorf("Failed to send message: %v", err)
				p.writeToFallbackFile(err.Msg) // 写入到降级文件
			}
		case <-p.shutdown:
			return
		}
	}
}

func (p *UserProducer) SendUserCreateMessage(ctx context.Context, user *v1.User) error {
	log.Debugf("[Producer] SendUserCreateMessage: username=%s", user.Name)
	return p.sendUserMessage(ctx, user, OperationCreate, UserCreateTopic)
}

func (p *UserProducer) SendUserUpdateMessage(ctx context.Context, user *v1.User) error {
	log.Debugf("[Producer] SendUserUpdateMessage: username=%s", user.Name)
	return p.sendUserMessage(ctx, user, OperationUpdate, UserUpdateTopic)
}

func (p *UserProducer) SendUserDeleteMessage(ctx context.Context, username string) error {
	log.Debugf("[Producer] SendUserDeleteMessage: username=%s", username)
	deleteData := map[string]interface{}{
		"username":   username,
		"deleted_at": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(deleteData)
	if err != nil {
		return errors.WithCode(code.ErrEncodingJSON, "failed to marshal delete message: %v", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: UserDeleteTopic,
		Key:   sarama.StringEncoder(username),
		Value: sarama.ByteEncoder(data),
		Headers: []sarama.RecordHeader{
			{Key: []byte(HeaderOperation), Value: []byte(OperationDelete)},
			{Key: []byte(HeaderOriginalTimestamp), Value: []byte(time.Now().Format(time.RFC3339))},
			{Key: []byte(HeaderRetryCount), Value: []byte("0")},
		},
	}

	p.producer.Input() <- msg
	return nil
}

// 新增：写入降级文件的方法
func (p *UserProducer) writeToFallbackFile(msg *sarama.ProducerMessage) {
	if p.fallbackDir == "" {
		log.Warnf("Fallback directory not configured. Message lost: key=%s", msg.Key)
		return
	}

	// 确保目录存在
	if err := os.MkdirAll(p.fallbackDir, 0755); err != nil {
		log.Errorf("Failed to create fallback directory %s: %v", p.fallbackDir, err)
		return
	}

	// 按天创建文件名
	fileName := fmt.Sprintf("%s.json", time.Now().Format("2006-01-02"))
	filePath := filepath.Join(p.fallbackDir, fileName)

	// 构造要写入的 JSON 对象
	value, _ := msg.Value.Encode()

	var key string
	if msg.Key != nil {
		encodedKey, _ := msg.Key.Encode()
		key = string(encodedKey)
	}

	fallbackEntry := map[string]interface{}{
		"topic":     msg.Topic,
		"key":       key,
		"value":     string(value),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// 序列化为 JSON
	jsonData, err := json.Marshal(fallbackEntry)
	if err != nil {
		log.Errorf("Failed to marshal fallback message to JSON: %v", err)
		return
	}

	// 以追加模式打开文件
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Failed to open fallback file %s: %v", filePath, err)
		return
	}
	defer file.Close()

	// 写入 JSON 数据，并在线末添加换行符
	if _, err := file.Write(append(jsonData, '\n')); err != nil {
		log.Errorf("Failed to write to fallback file %s: %v", filePath, err)
	} else {
		log.Infof("Message successfully written to fallback file: %s", filePath)
	}
}

func (p *UserProducer) sendUserMessage(ctx context.Context, user *v1.User, operation, topic string) error {
	log.Debugf("[Producer] sendUserMessage: username=%s, operation=%s, topic=%s", user.Name, operation, topic)

	if p.limiter != nil {
		if err := p.limiter.Wait(ctx); err != nil {
			return errors.WithCode(code.ErrRateLimitExceeded, "producer rate limit exceeded: %v", err)
		}
	}

	start := time.Now()
	var errSend error
	defer func() {
		metrics.RecordKafkaProducerOperation(topic, operation, time.Since(start).Seconds(), errSend, false)
	}()

	userData, err := json.Marshal(user)
	if err != nil {
		errSend = err
		log.Errorf("Failed to marshal user %s for topic %s, operation %s: %v", user.Name, topic, operation, err)
		return errors.WithCode(code.ErrEncodingJSON, "failed to marshal user message: %v", err)
	}

	now := time.Now()
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(strconv.FormatUint(user.ID, 10)),
		Value: sarama.ByteEncoder(userData),
		Headers: []sarama.RecordHeader{
			{Key: []byte(HeaderOperation), Value: []byte(operation)},
			{Key: []byte(HeaderOriginalTimestamp), Value: []byte(now.Format(time.RFC3339))},
			{Key: []byte(HeaderRetryCount), Value: []byte("0")},
		},
	}

	p.producer.Input() <- msg
	return nil
}

// sendToRetryTopic is called by the consumer to send a message to the retry topic.
// It needs to accept a kafka-go message and convert it to a sarama message.
func (p *UserProducer) sendToRetryTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	log.Warnf("[Producer] Forwarding to retry topic: key=%s, error=%s", string(msg.Key), errorInfo)

	// Convert kafka.Message to sarama.ProducerMessage
	saramaMsg := &sarama.ProducerMessage{
		Topic: UserRetryTopic,
		Key:   sarama.ByteEncoder(msg.Key),
		Value: sarama.ByteEncoder(msg.Value),
	}

	// Copy and update headers
	headers := make([]sarama.RecordHeader, 0, len(msg.Headers)+1)
	for _, h := range msg.Headers {
		headers = append(headers, sarama.RecordHeader{Key: []byte(h.Key), Value: h.Value})
	}
	headers = p.updateOrAddHeader(headers, HeaderRetryError, errorInfo)
	saramaMsg.Headers = headers

	// 使用 select 防止阻塞，并在无法立即发送时触发降级
	select {
	case p.producer.Input() <- saramaMsg:
		log.Debugf("Successfully enqueued message to retry topic for key: %s", string(msg.Key))
	default:
		log.Errorf("Failed to enqueue message to retry topic: producer input channel is full or closed. Triggering fallback.")
		p.writeToFallbackFile(saramaMsg)
	}

	return nil
}

// SendToDeadLetterTopic is called by the consumer to send a message to the dead-letter topic.
// It needs to accept a kafka-go message and convert it to a sarama message.
func (p *UserProducer) SendToDeadLetterTopic(ctx context.Context, msg kafka.Message, errorInfo string) error {
	log.Errorf("[Producer] Forwarding to dead-letter topic: key=%s, error=%s", string(msg.Key), errorInfo)

	// Convert kafka.Message to sarama.ProducerMessage
	saramaMsg := &sarama.ProducerMessage{
		Topic: UserDeadLetterTopic,
		Key:   sarama.ByteEncoder(msg.Key),
		Value: sarama.ByteEncoder(msg.Value),
	}

	// Copy and update headers
	headers := make([]sarama.RecordHeader, 0, len(msg.Headers)+2)
	for _, h := range msg.Headers {
		headers = append(headers, sarama.RecordHeader{Key: []byte(h.Key), Value: h.Value})
	}
	headers = p.updateOrAddHeader(headers, "deadletter-reason", errorInfo)
	headers = p.updateOrAddHeader(headers, "deadletter-timestamp", time.Now().Format(time.RFC3339))
	saramaMsg.Headers = headers

	p.producer.Input() <- saramaMsg
	return nil
}

func (p *UserProducer) Close() error {
	log.Infof("[Producer] Close called")
	close(p.shutdown) // Signal background goroutines to exit

	// Drain any remaining messages
	if p.producer != nil {
		// Note: AsyncClose does not block. The wg.Wait() below will ensure graceful shutdown.
		p.producer.AsyncClose()
	}

	p.wg.Wait() // Wait for goroutines to finish
	log.Infof("[Producer] Closed successfully")
	return nil
}

func (p *UserProducer) getOperationFromHeaders(headers []sarama.RecordHeader) string {
	for _, h := range headers {
		if string(h.Key) == HeaderOperation {
			return string(h.Value)
		}
	}
	return "unknown"
}

func (p *UserProducer) updateOrAddHeader(headers []sarama.RecordHeader, key, value string) []sarama.RecordHeader {
	log.Debugf("[Producer] updateOrAddHeader: key=%s, value=%s", key, value)
	if key == "" {
		panic("kafka header key cannot be empty string")
	}

	targetKeyLower := strings.ToLower(key)
	var newHeaders []sarama.RecordHeader
	foundTargetHeader := false

	for _, header := range headers {
		currentHeaderKeyLower := strings.ToLower(string(header.Key))
		if currentHeaderKeyLower == targetKeyLower {
			// Update existing header
			newHeaders = append(newHeaders, sarama.RecordHeader{
				Key:   []byte(key),
				Value: []byte(value),
			})
			foundTargetHeader = true
		} else {
			newHeaders = append(newHeaders, header)
		}
	}

	if !foundTargetHeader {
		newHeaders = append(newHeaders, sarama.RecordHeader{
			Key:   []byte(key),
			Value: []byte(value),
		})
	}
	return newHeaders
}
