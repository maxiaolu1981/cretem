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

func NewRetryConsumer(db *gorm.DB, redis *storage.RedisCluster, producer *UserProducer, kafkaOptions *options.KafkaOptions) *RetryConsumer {
	return &RetryConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        kafkaOptions.Brokers,
			Topic:          UserRetryTopic,
			GroupID:        "user-retry-group",
			MinBytes:       10e3,
			MaxBytes:       10e6,
			CommitInterval: 2 * time.Second,
			StartOffset:    kafka.FirstOffset,
		}),
		db:         db,
		redis:      redis,
		producer:   producer,
		maxRetries: kafkaOptions.MaxRetries,
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

	for {
		select {
		case <-ctx.Done():
			log.Infof("重试Worker %d: 停止消费", workerID)
			return
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

	log.Infof("处理重试创建: username=%s, 重试次数=%d, 上次错误=%s",
		user.Name, currentRetryCount, lastError)

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

	log.Infof("开始第%d次重试创建: username=%s", currentRetryCount+1, user.Name)

	// exists, err := rc.checkUserExists(ctx, user.Name)
	// if err != nil {
	// 	return rc.handleProcessingError(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	// }
	// if exists {
	// 	log.Debugf("用户已存在，重试创建成功: username=%s", user.Name)
	// 	return nil
	// }

	if err := rc.createUserInDB(ctx, &user); err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "检查用户存在性失败: "+err.Error())
	}

	rc.setUserCache(ctx, &user)

	log.Infof("第%d次重试创建成功: username=%s", currentRetryCount+1, user.Name)
	return nil
}

func (rc *RetryConsumer) processRetryUpdate(ctx context.Context, msg kafka.Message) error {
	var user v1.User
	if err := json.Unmarshal(msg.Value, &user); err != nil {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "POISON_MESSAGE_IN_RETRY: "+err.Error())
	}

	currentRetryCount, nextRetryTime, lastError := rc.parseRetryHeaders(msg.Headers)

	log.Infof("处理重试更新: username=%s, 重试次数=%d", user.Name, currentRetryCount)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		time.Sleep(time.Until(nextRetryTime))
	}

	exists, err := rc.checkUserExists(ctx, user.Name)
	if err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}
	if !exists {
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "USER_NOT_EXISTS_FOR_UPDATE")
	}

	if err := rc.updateUserInDB(ctx, &user); err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}

	rc.setUserCache(ctx, &user)

	log.Infof("第%d次重试更新成功: username=%s", currentRetryCount+1, user.Name)
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

	log.Infof("处理重试删除: username=%s, 重试次数=%d", deleteRequest.Username, currentRetryCount)

	if currentRetryCount >= rc.maxRetries {
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, lastError))
	}

	if time.Now().Before(nextRetryTime) {
		time.Sleep(time.Until(nextRetryTime))
	}

	exists, err := rc.checkUserExists(ctx, deleteRequest.Username)
	if err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}
	if !exists {
		log.Debugf("用户已不存在，重试删除成功: username=%s", deleteRequest.Username)
		return nil
	}

	if err := rc.deleteUserFromDB(ctx, deleteRequest.Username); err != nil {
		return rc.handleProcessingError(ctx, msg, currentRetryCount, "错误信息: "+err.Error())
	}

	if err := rc.deleteUserCache(ctx, deleteRequest.Username); err != nil {
		log.Errorw("重试删除成功但缓存删除失败", "username", deleteRequest.Username, "error", err)
	}

	// 布隆过滤器不支持删除操作
	log.Debugf("重试删除成功，布隆过滤器需要等待重建: username=%s", deleteRequest.Username)

	log.Infof("第%d次重试删除成功: username=%s", currentRetryCount+1, deleteRequest.Username)
	return nil
}

// handleProcessingError 处理错误，决定是重试还是进入死信队列
func (rc *RetryConsumer) handleProcessingError(ctx context.Context, msg kafka.Message, currentRetryCount int, errorInfo string) error {
	// 如果是致命错误，直接进入死信队列
	if shouldGoToDeadLetterImmediately(errorInfo) {
		log.Warnf("致命错误，直接进入死信队列: %s", errorInfo)
		return rc.producer.SendToDeadLetterTopic(ctx, msg, "FATAL_ERROR: "+errorInfo)
	}

	// 否则进行智能重试
	return rc.prepareNextRetry(ctx, msg, currentRetryCount, errorInfo)
}

// shouldGoToDeadLetterImmediately 判断是否应该直接进入死信队列
func shouldGoToDeadLetterImmediately(errorInfo string) bool {
	// 这些错误不需要重试，直接进入死信队列
	fatalErrors := []string{
		"invalid json",
		"unknown operation",
		"permission denied",
		"not found",
		"invalid format",
		"poison message",
	}

	lowerError := strings.ToLower(errorInfo)
	for _, fatalError := range fatalErrors {
		if strings.Contains(lowerError, fatalError) {
			return true
		}
	}
	return false
}

// 优化后的prepareNextRetry方法
func (rc *RetryConsumer) prepareNextRetry(ctx context.Context, msg kafka.Message, currentRetryCount int, errorInfo string) error {
	log.Debugf("开始处理消息: key=%s, headers=%v", string(msg.Key), msg.Headers)
	log.Debugf("原始消息Headers:")
	newRetryCount := currentRetryCount + 1

	// 检查是否达到最大重试次数
	if newRetryCount > rc.maxRetries {
		log.Warnf("达到最大重试次数(%d)，发送到死信队列: error=%s", rc.maxRetries, errorInfo)
		return rc.producer.SendToDeadLetterTopic(ctx, msg,
			fmt.Sprintf("MAX_RETRIES_EXCEEDED(%d): %s", rc.maxRetries, errorInfo))
	}

	// 根据错误类型决定重试策略
	var nextRetryDelay time.Duration // 下一次重试要等多久
	var retryStrategy string         // 记录用了什么策略（方便日志/监控）

	switch {
	case isDBConnectionError(errorInfo):
		// 数据库连接问题：指数退避 + 随机抖动
		// 策略：指数退避+随机抖动（失败次数越多，等越久，且加随机值避免“重试风暴”）
		// 2^current * 基础延迟（如基础1秒，第1次2秒，第2次4秒...）
		baseDelay := time.Duration(1<<currentRetryCount) * rc.kafkaOptions.BaseRetryDelay
		// 加0~50%的随机延迟（比如4秒基础延迟，加0~2秒随机值）
		jitter := time.Duration(rand.Int63n(int64(baseDelay / 2))) // 最多50%的抖动
		nextRetryDelay = baseDelay + jitter
		retryStrategy = "指数退避(连接问题)"
		// 情况2：数据库死锁（瞬时冲突，很快恢复）
	case isDBDeadlockError(errorInfo):
		// 死锁问题：快速重试，短延迟
		// 策略：快速重试（100ms后就试，死锁通常是临时的）
		nextRetryDelay = 100 * time.Millisecond
		retryStrategy = "快速重试(死锁)"
		// 情况3：重复键错误（如唯一索引冲突，可能是并发导致）
	case isDuplicateKeyError(errorInfo):
		// 重复键错误：可能是幂等问题，中等延迟
		nextRetryDelay = 2 * time.Second
		retryStrategy = "中等延迟(重复键)"
		// 情况4：其他临时错误（如资源暂时耗尽
	case isTemporaryError(errorInfo):
		// 临时性错误：线性增长
		// 策略：线性增长（第1次1秒，第2次2秒...比指数平缓）
		nextRetryDelay = time.Duration(currentRetryCount+1) * rc.kafkaOptions.BaseRetryDelay
		retryStrategy = "线性增长(临时错误)"

	default:
		// 其他错误：默认指数退避
		nextRetryDelay = time.Duration(1<<currentRetryCount) * rc.kafkaOptions.BaseRetryDelay
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
	// 调试：打印复制后的headers
	log.Debugf("复制后的Headers:")
	for i, header := range newHeaders {
		log.Debugf("  [%d] %s: %s", i, header.Key, string(header.Value))
	}

	// 更新重试相关的headers
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderRetryCount, strconv.Itoa(newRetryCount))
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderNextRetryTS, nextRetryTime.Format(time.RFC3339))
	newHeaders = rc.updateOrAddHeader(newHeaders, HeaderRetryError, errorInfo)
	newHeaders = rc.updateOrAddHeader(newHeaders, "retry-strategy", retryStrategy)
	// 调试：打印最终headers
	log.Debugf("最终Headers:")
	for i, header := range newHeaders {
		log.Debugf("  [%d] %s: %s", i, header.Key, string(header.Value))
	}
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
		Where("name = ?", username).
		Delete(&v1.User{}).Error
}

func (rc *RetryConsumer) checkUserExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := rc.db.WithContext(ctx).Model(&v1.User{}).
		Where("name = ?", username).
		Count(&count).Error
	return count > 0, err
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

// updateOrAddHeader 更新或添加header
// updateOrAddHeader 安全地更新或添加Kafka消息头（忽略Key的大小写，不修改原始切片）
// 参数：
//
//	headers: 原始消息头切片（可为nil或空）
//	key:     要操作的消息头键（不可为空字符串，忽略大小写，如"Retry-Count"和"retry-count"视为同一键）
//	value:   要设置的消息头值（可为空字符串，会转为[]byte存储）
//
// 返回值：
//
//	新的消息头切片（与原始切片完全独立）

// updateOrAddHeader 安全更新/添加Kafka消息头（核心特性：1.忽略Key大小写 2.只保留一个目标头 3.不修改原始切片）
// 参数：
//
//	headers: 原始消息头切片（可为nil/空，自动兼容）
//	key:     目标头Key（不可为空，匹配时忽略大小写，如"Retry-Error"与"retry-error"视为同一Key）
//	value:   目标头最新Value（可为空，自动转为[]byte）
//
// 返回值：
//
//	新消息头切片（与原始切片完全独立，仅含一个目标头，其余非目标头保持不变）
//
// 异常：
//
//	若key为空字符串，直接panic（空Key无业务意义，会导致头逻辑混乱）
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
