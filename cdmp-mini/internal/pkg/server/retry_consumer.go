// internal/pkg/server/retry_consumer.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/options"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"
)

type RetryConsumer struct {
	reader       *kafka.Reader
	db           *gorm.DB
	redis        *storage.RedisCluster
	producer     *UserProducer
	maxRetries   int
	kafkaOptions *options.KafkaOptions
}

func NewRetryConsumer(db *gorm.DB, redis *storage.RedisCluster, producer *UserProducer, kafkaOptions *options.KafkaOptions, topic, groupid string) *RetryConsumer {
	return &RetryConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        kafkaOptions.Brokers,
			Topic:          topic,
			GroupID:        groupid,
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: 2 * time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		db:           db,
		redis:        redis,
		producer:     producer,
		maxRetries:   kafkaOptions.MaxRetries,
		kafkaOptions: kafkaOptions,
	}
}

func (rc *RetryConsumer) Close() error {
	if rc.reader != nil {
		return rc.reader.Close()
	}
	return nil
}

func (rc *RetryConsumer) StartConsuming(ctx context.Context, workerCount int) {
	log.Infof("启动重试消费者，worker数量: %d", workerCount)

	var wg sync.WaitGroup

	// 为每个worker启动一个goroutine
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rc.retryWorker(ctx, workerID)
		}(i)
	}

	wg.Wait()
}

func (rc *RetryConsumer) retryWorker(ctx context.Context, workerID int) {
	log.Infof("启动重试消费者Worker %d", workerID)

	// 添加健康检查计数器
	healthCheckTicker := time.NewTicker(30 * time.Second)
	defer healthCheckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Infof("重试Worker %d: 停止消费", workerID)
			return
		case <-healthCheckTicker.C:
			stats := rc.reader.Stats()
			log.Infof("Worker %d: 消费者状态 - Lag: %d, 错误数: %d", workerID, stats.Lag, stats.Errors)

			// 添加告警
			if stats.Lag > 1000 {
				log.Errorf("Worker %d: 消费延迟过高! Lag: %d", workerID, stats.Lag)
			}
			if stats.Errors > 10 {
				log.Errorf("Worker %d: 错误数过多! Errors: %d", workerID, stats.Errors)
			}
		default:
			msg, err := rc.reader.FetchMessage(ctx)
			if err != nil {
				log.Errorf("重试Worker %d: 获取消息失败: %v", workerID, err)
				continue
			}

			if err := rc.processRetryMessage(ctx, msg); err != nil {
				log.Errorf("重试Worker %d: 处理消息失败: %v", workerID, err)
				continue
			}

			if err := rc.reader.CommitMessages(ctx, msg); err != nil {
				log.Errorf("重试Worker %d: 提交偏移量失败: %v", workerID, err)
			}
		}
	}
}

func (rc *RetryConsumer) processRetryMessage(ctx context.Context, msg kafka.Message) error {

	operation := rc.getOperationFromHeaders(msg.Headers)

	switch operation {
	case OperationCreate:
		return rc.processRetryCreate(ctx, msg)
	case OperationUpdate:
		return rc.processRetryUpdate(ctx, msg)
	case OperationDelete:
		return rc.processRetryDelete(ctx, msg)
	default:
		log.Errorf("重试消息未知操作类型: %s", operation)
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "UNKNOWN_OPERATION_IN_RETRY: "+operation)
	}
}

func (rc *RetryConsumer) getOperationFromHeaders(headers []kafka.Header) string {
	for _, header := range headers {
		if header.Key == HeaderOperation {
			return string(header.Value)
		}
	}
	return OperationCreate
}

func (rc *RetryConsumer) parseRetryHeaders(headers []kafka.Header) (int, time.Time, string) {
	var (
		currentRetryCount int = 0
		nextRetryTime     time.Time
		lastError         string = "未知错误"
	)

	for _, header := range headers {
		switch header.Key {
		case HeaderRetryCount:
			if count, err := strconv.Atoi(string(header.Value)); err == nil {
				currentRetryCount = count
			}
		case HeaderNextRetryTS:
			if t, err := time.Parse(time.RFC3339, string(header.Value)); err == nil {
				nextRetryTime = t
			}
		case HeaderRetryError:
			lastError = string(header.Value)
		}
	}

	// 如果没有找到下次重试时间，设置一个默认值
	if nextRetryTime.IsZero() {
		nextRetryTime = time.Now().Add(10 * time.Second)
		log.Warnf("未找到下次重试时间，使用默认值: %v", nextRetryTime)
	}

	return currentRetryCount, nextRetryTime, lastError
}

func (rc *RetryConsumer) processRetryCreate(ctx context.Context, msg kafka.Message) error {
	log.Debugf("开始处理重试创建消息: key=%s, headers=%+v", string(msg.Key), msg.Headers)
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		log.Errorf("重试消息解析失败: %v, 原始消息: %s", err, string(msg.Value))
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	if currentRetryCount >= rc.maxRetries {
		log.Warnf("创建消息达到最大重试次数(%d), 发送到死信队列: username=%s",
			rc.maxRetries, user.Name)
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		remaining := time.Until(nextRetryTime)
		log.Debugf("等待重试时间到达: username=%s, 剩余=%v", user.Name, remaining)
		select {
		case <-time.After(remaining):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err := rc.createUserInDB(ctx, &user); err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	}

	rc.setUserCache(ctx, &user)

	return nil
}

func (rc *RetryConsumer) processRetryUpdate(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		time.Sleep(time.Until(nextRetryTime))
	}

	if err := rc.updateUserInDB(ctx, &user); err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}

	rc.setUserCache(ctx, &user)

	return nil
}

// internal/pkg/server/retry_consumer.go
// 修改 processRetryDelete 方法
func (rc *RetryConsumer) processRetryDelete(ctx context.Context, msg kafka.Message) error {
	var deleteRequest struct {
		Username  string `json:"username"`
		DeletedAt string `json:"deleted_at"`
	}

	if err := json.Unmarshal(msg.Value, &deleteRequest); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	// 🔴 优化：如果重试时间未到，直接重新入队而不是阻塞
	if time.Now().Before(nextRetryTime) {
		log.Debugf("重试时间未到，重新入队等待: username=%s, delay=%v",
			deleteRequest.Username, time.Until(nextRetryTime))
		return rc.prepareNextRetry(ctx, msg, currentRetryCount, lastError)
	}

	if err := rc.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		// 检查是否为可忽略的错误
		if rc.isIgnorableDeleteError(err) {
			log.Warnf("删除操作遇到可忽略错误，直接提交: username=%s, error=%v",
				deleteRequest.Username, err)
			rc.deleteUserCache(ctx, deleteRequest.Username)
			return nil
		}
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}

	if err := rc.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("重试删除成功但缓存删除失败", "username", deleteRequest.Username, "error", err)
	}
	return nil
}

// 新增：判断是否为可忽略的删除错误
func (rc *RetryConsumer) isIgnorableDeleteError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	ignorableErrors := []string{
		"definer",          // DEFINER权限错误
		"does not exist",   // 数据不存在
		"not found",        // 数据不存在
		"record not found", // 数据不存在
		"duplicate entry",  // 重复数据
		"1062",             // MySQL重复键
	}

	lowerError := strings.ToLower(errStr)
	for _, ignorableError := range ignorableErrors {
		if strings.Contains(lowerError, ignorableError) {
			return true
		}
	}
	return false
}

// handleProcessingError 处理错误，决定是重试还是进入死信队列
func (rc *RetryConsumer) handleProcessingError(ctx context.Context, msg kafka.Message, currentRetryCount int, errorInfo string) error {
	// 1. 可以直接忽略的错误 - 直接提交
	if shouldIgnoreError(errorInfo) {
		log.Warnf("忽略错误，直接提交消息: %s", errorInfo)
		return nil // 直接返回nil，外层会提交偏移量
	}

	// 2. 需要人工干预的错误 - 发送到死信队列
	if shouldGoToDeadLetterImmediately(errorInfo) {
		log.Warnf("致命错误，发送到死信队列: %s", errorInfo)
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "FATAL_ERROR: "+errorInfo)
	}

	// 3. 可以重试的错误 - 准备下次重试
	return rc.prepareNextRetry(ctx, msg, currentRetryCount, errorInfo)
}

// shouldIgnoreError 判断是否可以直接忽略的错误
func shouldIgnoreError(errorInfo string) bool {
	ignorableErrors := []string{
		"definer",            // DEFINER权限错误
		"does not exist",     // 数据不存在
		"not found",          // 数据不存在
		"record not found",   // 数据不存在
		"duplicate entry",    // 重复数据（幂等性）
		"user already exist", // 用户已存在
		"1062",               // MySQL重复键错误
	}

	lowerError := strings.ToLower(errorInfo)
	for _, ignorableError := range ignorableErrors {
		if strings.Contains(lowerError, ignorableError) {
			return true
		}
	}
	return false
}

// 更新 shouldGoToDeadLetterImmediately，移除已归类到忽略的错误
func shouldGoToDeadLetterImmediately(errorInfo string) bool {
	fatalErrors := []string{
		"invalid json",      // 消息格式错误
		"unmarshal error",   // 反序列化错误
		"unknown operation", // 未知操作类型
		"poison message",    // 毒药消息
		"permission denied", // 真正的权限错误（不是DEFINER）
	}

	lowerError := strings.ToLower(errorInfo)
	for _, fatalError := range fatalErrors {
		if strings.Contains(lowerError, fatalError) {
			return true
		}
	}
	return false
}

func (rc *RetryConsumer) prepareNextRetry(ctx context.Context, msg kafka.Message, currentRetryCount int, errorInfo string) error {
	// 确保 currentRetryCount 不会导致负数位移
	if currentRetryCount < 0 {
		currentRetryCount = 0
	}

	newRetryCount := currentRetryCount + 1

	// 检查是否达到最大重试次数
	if newRetryCount > rc.maxRetries {
		log.Warnf("达到最大重试次数(%d)，发送到死信队列: error=%s", rc.maxRetries, errorInfo)
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, errorInfo))
	}

	// 根据错误类型决定重试策略
	var nextRetryDelay time.Duration
	var retryStrategy string

	switch {
	case isDBConnectionError(errorInfo):
		// 安全的指数退避计算
		baseDelay := rc.kafkaOptions.BaseRetryDelay
		if currentRetryCount > 0 {
			baseDelay = rc.kafkaOptions.BaseRetryDelay * time.Duration(1<<(currentRetryCount-1))
		}
		jitter := time.Duration(rand.Int63n(int64(baseDelay / 2)))
		nextRetryDelay = baseDelay + jitter
		retryStrategy = "指数退避(连接问题)"

	case isDBDeadlockError(errorInfo):
		nextRetryDelay = 100 * time.Millisecond
		retryStrategy = "快速重试(死锁)"

	case isDuplicateKeyError(errorInfo):
		nextRetryDelay = 2 * time.Second
		retryStrategy = "中等延迟(重复键)"

	case isTemporaryError(errorInfo):
		// 线性增长：n * baseDelay
		nextRetryDelay = time.Duration(currentRetryCount+1) * rc.kafkaOptions.BaseRetryDelay
		retryStrategy = "线性增长(临时错误)"

	default:
		// 默认指数退避 - 确保不会负数位移
		baseDelay := rc.kafkaOptions.BaseRetryDelay
		if currentRetryCount > 0 {
			baseDelay = rc.kafkaOptions.BaseRetryDelay * time.Duration(1<<(currentRetryCount-1))
		}
		nextRetryDelay = baseDelay
		retryStrategy = "指数退避(默认)"
	}

	// 应用最大延迟限制
	if nextRetryDelay > rc.kafkaOptions.MaxRetryDelay {
		nextRetryDelay = rc.kafkaOptions.MaxRetryDelay
	}

	nextRetryTime := time.Now().Add(nextRetryDelay)

	// 创建新的headers数组
	newHeaders := make([]kafka.Header, len(msg.Headers))
	copy(newHeaders, msg.Headers)

	// 更新重试相关的headers
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderRetryCount, strconv.Itoa(newRetryCount))
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderNextRetryTS, nextRetryTime.Format(time.RFC3339))
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderRetryError, errorInfo)
	newHeaders = rc.updateOrAddHeader(newHeaders, "retry-strategy", retryStrategy)

	// 创建重试消息
	retryMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: newHeaders,
		Time:    time.Now(),
	}

	log.Warnf("准备第%d次重试[%s]: delay=%v, error=%s",
		newRetryCount, retryStrategy, nextRetryDelay, errorInfo)

	return rc.producer.sendToRetryTopic(ctx, retryMsg, errorInfo)
}

// 错误类型判断函数
func isDBConnectionError(errorInfo string) bool {
	return strings.Contains(errorInfo, "connection") ||
		strings.Contains(errorInfo, "timeout") ||
		strings.Contains(errorInfo, "network") ||
		strings.Contains(errorInfo, "refused")
}

func isDBDeadlockError(errorInfo string) bool {
	return strings.Contains(errorInfo, "deadlock") ||
		strings.Contains(errorInfo, "Deadlock") ||
		strings.Contains(errorInfo, "1213") || // MySQL死锁错误码
		strings.Contains(errorInfo, "40001") // SQLServer死锁错误码
}

func isDuplicateKeyError(errorInfo string) bool {
	return strings.Contains(errorInfo, "1062") || // MySQL重复键
		strings.Contains(errorInfo, "Duplicate") ||
		strings.Contains(errorInfo, "unique constraint")
}

func isTemporaryError(errorInfo string) bool {
	return strings.Contains(errorInfo, "temporary") ||
		strings.Contains(errorInfo, "busy") ||
		strings.Contains(errorInfo, "lock")
}

func (rc *RetryConsumer) createUserInDB(ctx context.Context, user *v1.User) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	return rc.db.WithContext(ctx).Create(user).Error
}

func (rc *RetryConsumer) updateUserInDB(ctx context.Context, user *v1.User) error {
	user.UpdatedAt = time.Now()
	return rc.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", user.Name).
		Updates(map[string]interface{}{
			"email":      user.Email,
			"password":   user.Password,
			"status":     user.Status,
			"updated_at": user.UpdatedAt,
		}).Error
}

func (rc *RetryConsumer) deleteUserFromDB(ctx context.Context, username string) error {
	return rc.db.WithContext(ctx).
		Where("name = ? and status = 1", username).
		Delete(&v1.User{}).Error
}

func (rc *RetryConsumer) setUserCache(ctx context.Context, user *v1.User) error {
	startTime := time.Now()
	var operationErr error
	defer func() {
		metrics.RecordRedisOperation("set", float64(time.Since(startTime).Seconds()), operationErr)
	}()
	cacheKey := fmt.Sprintf("user:%s", user.Name)
	data, err := json.Marshal(user)
	if err != nil {
		operationErr = err
		log.L(ctx).Errorw("用户数据序列化失败", "username", user.Name, "error", err)
		return operationErr
	}
	operationErr = rc.redis.SetKey(ctx, cacheKey, string(data), 24*time.Hour)
	if operationErr != nil {
		log.L(ctx).Errorw("缓存写入失败", "username", user.Name, "error", operationErr)
	}
	return operationErr
}

func (rc *RetryConsumer) deleteUserCache(ctx context.Context, username string) error {
	cacheKey := fmt.Sprintf("user:%s", username)
	_, err := rc.redis.DeleteKey(ctx, cacheKey)
	return err
}

// 若key为空字符串，直接panic（空Key无业务意义，会导致头逻辑混乱）
func (rc *RetryConsumer) updateOrAddHeader(headers []kafka.Header, key, value string) []kafka.Header {
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
