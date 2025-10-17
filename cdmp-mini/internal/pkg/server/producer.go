package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

type fallbackMessage struct {
	Topic     string `json:"topic"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value"`
	Timestamp string `json:"timestamp"`
	Attempts  int    `json:"attempts"`
}

func NewUserProducer(
	options *options.KafkaOptions,
	limiter *ratelimiter.RateLimiterController,
	fallbackDir string,
) (*UserProducer, error) {
	log.Infof("[Producer] Initializing with options: %+v", options)

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.RequiredAcks(options.RequiredAcks)

	compressionCodec, err := parseCompressionCodec(options.ProducerCompression)
	if err != nil {
		return nil, fmt.Errorf("invalid compression codec: %w", err)
	}
	config.Producer.Compression = compressionCodec

	config.Producer.Flush.Frequency = options.FlushFrequency
	config.Producer.Flush.MaxMessages = options.FlushMaxMessages
	config.Producer.Return.Successes = options.ProducerReturnSuccesses
	config.Producer.Return.Errors = options.ProducerReturnErrors

	if options.ChannelBufferSize > 0 {
		config.ChannelBufferSize = options.ChannelBufferSize
	}

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

	if fallbackDir != "" && options.FallbackRetryEnabled {
		up.wg.Add(1)
		go up.runFallbackCompensator()
	}

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

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		err := errors.WithCode(code.ErrKafkaFailed, "context cancelled while sending delete message: %v", ctx.Err())
		return err
	default:
		log.Errorf("Failed to enqueue delete message for username=%s: producer input channel is full. Triggering fallback.", username)
		p.writeToFallbackFile(msg)
		return errors.WithCode(code.ErrKafkaFailed, "producer input channel is full, delete message written to fallback")
	}
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

	entry := fallbackMessage{
		Topic:     msg.Topic,
		Key:       key,
		Value:     string(value),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Attempts:  0,
	}

	// 序列化为 JSON
	jsonData, err := json.Marshal(entry)
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

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		errSend = errors.WithCode(code.ErrKafkaFailed, "context cancelled while sending user message: %v", ctx.Err())
		return errSend
	default:
		log.Errorf("Failed to enqueue user message: operation=%s topic=%s username=%s. Triggering fallback.", operation, topic, user.Name)
		p.writeToFallbackFile(msg)
		errSend = errors.WithCode(code.ErrKafkaFailed, "producer input channel is full, user message written to fallback")
		return errSend
	}
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

	select {
	case p.producer.Input() <- saramaMsg:
		return nil
	default:
		log.Errorf("Failed to enqueue message to dead-letter topic: producer input channel is full or closed. Triggering fallback.")
		p.writeToFallbackFile(saramaMsg)
	}

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

func (p *UserProducer) runFallbackCompensator() {
	defer p.wg.Done()
	logger := log.WithName("fallback-compensator")
	logger.Info("Compensator started")
	ticker := time.NewTicker(p.kafkaOptions.FallbackRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdown:
			logger.Info("Compensator shutting down")
			return
		case <-ticker.C:
			p.processFallbackFiles(logger)
		}
	}
}

func (p *UserProducer) processFallbackFiles(logger log.Logger) {
	if p.fallbackDir == "" {
		return
	}

	files, err := filepath.Glob(filepath.Join(p.fallbackDir, "*.json"))
	if err != nil {
		logger.Errorf("Failed to list fallback files: %v", err)
		return
	}

	if len(files) == 0 {
		return
	}

	sort.Strings(files)

	processed := 0
	maxBatch := p.kafkaOptions.FallbackRetryBatchSize

	for _, filePath := range files {
		if maxBatch > 0 && processed >= maxBatch {
			return
		}

		count, err := p.processFallbackFile(logger, filePath, maxBatch-processed)
		if err != nil {
			logger.Errorf("Failed to process fallback file %s: %v", filePath, err)
		}
		processed += count
	}
}

func (p *UserProducer) processFallbackFile(logger log.Logger, filePath string, remainingQuota int) (int, error) {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	tempPath := filePath + ".tmp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer tempFile.Close()
	writer := bufio.NewWriter(tempFile)
	defer writer.Flush()

	retryMax := p.kafkaOptions.FallbackRetryMaxAttempts
	processed := 0

	for scanner.Scan() {
		if remainingQuota > 0 && processed >= remainingQuota {
			// Copy remaining entries as-is
			if _, err := writer.Write(scanner.Bytes()); err != nil {
				return processed, err
			}
			if _, err := writer.WriteString("\n"); err != nil {
				return processed, err
			}
			continue
		}

		line := scanner.Bytes()
		var entry fallbackMessage
		if err := json.Unmarshal(line, &entry); err != nil {
			logger.Errorf("Invalid fallback entry in %s: %v", filePath, err)
			continue
		}

		// Skip if attempts already exceed max retry limit
		if retryMax > 0 && entry.Attempts >= retryMax {
			logger.Warnf("Discarding fallback message after max attempts, topic=%s key=%s", entry.Topic, entry.Key)
			continue
		}

		if err := p.publishFallbackEntry(entry); err != nil {
			entry.Attempts++
			reEncoded, marshalErr := json.Marshal(entry)
			if marshalErr != nil {
				logger.Errorf("Failed to re-marshal fallback entry: %v", marshalErr)
				continue
			}
			if _, err := writer.Write(reEncoded); err != nil {
				return processed, err
			}
			if _, err := writer.WriteString("\n"); err != nil {
				return processed, err
			}
			continue
		}

		processed++
	}

	if err := scanner.Err(); err != nil {
		return processed, err
	}

	// Replace original file with temp file
	if err := os.Rename(tempPath, filePath); err != nil {
		return processed, err
	}

	return processed, nil
}

func (p *UserProducer) publishFallbackEntry(entry fallbackMessage) error {
	msg := &sarama.ProducerMessage{
		Topic: entry.Topic,
		Value: sarama.ByteEncoder(entry.Value),
	}

	if entry.Key != "" {
		msg.Key = sarama.StringEncoder(entry.Key)
	}

	msg.Headers = []sarama.RecordHeader{
		{Key: []byte(HeaderRetryCount), Value: []byte(strconv.Itoa(entry.Attempts))},
	}

	select {
	case <-p.shutdown:
		return fmt.Errorf("producer shutting down")
	case p.producer.Input() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("enqueue timeout")
	}
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

func parseCompressionCodec(codec string) (sarama.CompressionCodec, error) {
	switch strings.ToLower(codec) {
	case "", "none":
		return sarama.CompressionNone, nil
	case "snappy":
		return sarama.CompressionSnappy, nil
	case "gzip":
		return sarama.CompressionGZIP, nil
	case "lz4":
		return sarama.CompressionLZ4, nil
	case "zstd":
		return sarama.CompressionZSTD, nil
	default:
		return sarama.CompressionNone, fmt.Errorf("unsupported compression codec %q", codec)
	}
}
