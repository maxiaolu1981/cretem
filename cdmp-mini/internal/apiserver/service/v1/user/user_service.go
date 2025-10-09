package user

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/go-redis/redis/v8"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"golang.org/x/sync/singleflight"
)

const (
	RATE_LIMIT_PREVENTION = "rate_limit_prevention"
)

type UserService struct {
	Store    interfaces.Factory
	Redis    *storage.RedisCluster
	Options  *options.Options
	Producer producer.MessageProducer
	group    singleflight.Group
}

// NewUserService 创建用户服务实例
func NewUserService(store interfaces.Factory, redis *storage.RedisCluster, opts *options.Options, producer producer.MessageProducer) *UserService {
	return &UserService{
		Store:    store,
		Redis:    redis,
		Options:  opts,
		Producer: producer,
	}
}

type UserSrv interface {
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error
	Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions, opt *options.Options) error
	DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions, opt *options.Options) error
	Get(ctx context.Context, username string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error)
	List(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error)
	ListWithBadPerformance(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error)
	ChangePassword(ctx context.Context, user *v1.User, opt *options.Options) error
}

// getFromCache 从Redis获取缓存数据
func (u *UserService) getFromCache(ctx context.Context, cacheKey string) (*v1.User, bool, error) {
	startTime := time.Now()
	var operationErr error
	var cacheHit bool

	defer func() {
		metrics.RecordRedisOperation("get", time.Since(startTime).Seconds(), operationErr)
	}()

	log.Debugf("尝试从缓存获取用户数据: key=%s", cacheKey)
	data, err := u.Redis.GetKey(ctx, cacheKey)
	if err != nil {
		operationErr = err
		if errors.Is(err, redis.Nil) {
			log.Debugf("未进行缓存缓 key=%s", cacheKey)
			return nil, false, nil
		}
		log.Errorf("redis服务失败: key=%s, err=%v", cacheKey, err)
		return nil, false, err
	}

	// 初始化完整的 User 对象
	user := v1.User{
		ObjectMeta: metav1.ObjectMeta{},
	}
	//查找到防穿透记录
	if data == RATE_LIMIT_PREVENTION {
		log.Debugf("空值缓存命中: key=%s", cacheKey)
		user.Name = RATE_LIMIT_PREVENTION
		user.Status = -1
		cacheHit = true
	} else {
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			operationErr = err
			return nil, false, errors.WithCode(code.ErrDecodingFailed, "数据解码失败")
		}
		log.Debugf("从缓存中查询到用户:%s: key=%s", user.Name, cacheKey)
		cacheHit = true
	}

	// 确保返回的对象是有效的
	if user.Name == "" {
		return nil, cacheHit, errors.New("无效的用户数据")
	}
	return &user, cacheHit, nil
}

// getUserFromDBAndSetCache 带缓存的用户查询核心逻辑
func (u *UserService) getUserFromDBAndSetCache(ctx context.Context, username string) (*v1.User, error) {

	// 1. 查询数据库
	user, err := u.Store.Users().Get(ctx, username, metav1.GetOptions{}, u.Options)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			metrics.DBQueries.WithLabelValues("not_found").Inc()
			// 用户不存在，缓存空值（防止缓存击穿）
			if err := u.cacheNullValue(username); err != nil {
				log.Errorf("缓存设置失败", "error", err.Error())
			}

			//	log.Debugf("设置用户%s缓存成功", username)
			return &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: RATE_LIMIT_PREVENTION, // 直接赋值
				},
			}, nil
		}
		return nil, err
	}

	//写入缓存（带随机过期时间防雪崩）
	u.setUserCache(ctx, username, user)

	logger.Debugf("为用户%s设置缓存成功", username)
	return user, nil
}

// setUserCache 设置用户缓存
func (u *UserService) setUserCache(ctx context.Context, username string, user *v1.User) error {
	startTime := time.Now()
	var operationErr error

	defer func() {
		metrics.RecordRedisOperation("set", float64(time.Since(startTime).Seconds()), operationErr)
	}()

	data, err := json.Marshal(user)
	if err != nil {
		operationErr = err
		log.L(ctx).Errorf("用户数据序列化失败", "error", err.Error())
		return errors.Wrap(err, "用户数据序列化失败")
	}

	// 基础过期时间 + 随机时间防雪崩
	baseExpire := 1 * time.Hour
	randomExpire := time.Duration(rand.Intn(300)) * time.Second
	expireTime := baseExpire + randomExpire
	cacheKey := u.generateUserCacheKey(username)
	operationErr = u.Redis.SetKey(ctx, cacheKey, string(data), expireTime)
	if operationErr != nil {
		log.L(ctx).Errorf("缓存写入失败", "error", operationErr.Error())
		return operationErr
	}
	return nil
}

// cacheNullValue 缓存空值（防穿透）
func (u *UserService) cacheNullValue(username string) error {
	// 缓存一个短期空值，防止穿透
	ctx, cancel := context.WithTimeout(context.Background(), u.Options.RedisOptions.Timeout)
	defer cancel()

	cacheKey := u.generateUserCacheKey(username)

	baseExpire := 5 * time.Minute
	randomExpire := time.Duration(rand.Intn(60)) * time.Second
	expireTime := baseExpire + randomExpire

	_, err := u.Redis.SetNX(ctx, cacheKey, RATE_LIMIT_PREVENTION, expireTime)
	return err
}

func (u *UserService) generateUserCacheKey(username string) string {
	return fmt.Sprintf("user:%s", username)
}

// 从缓存和数据库查询用户是否存在
// 通用重试工具

func (u *UserService) checkUserExist(ctx context.Context, username string, forceRefresh bool) (*v1.User, error) {
	// 先尝试无锁查询缓存（大部分请求应该在这里返回）
	user, found, err := u.tryGetFromCache(ctx, username)
	if err != nil {
		log.Errorf("缓存查询异常，继续流程", "error", err.Error(), "username", username)
		metrics.CacheErrors.WithLabelValues("query_failed", "get").Inc()
	}
	if err == nil && found && user != nil {
		if user.Name == RATE_LIMIT_PREVENTION {
			if !forceRefresh {
				return user, nil
			}
			log.Debugf("命中负缓存, 强制回源校验 username=%s", username)
		} else {
			return user, nil
		}
	}

	// 缓存未命中，重试DB查询（带独立ctx）
	result, err := util.RetryWithBackoff(3, isRetryableError, func() (interface{}, error) {
		dbCtx, cancel := context.WithTimeout(context.Background(), time.Second) // 独立1s超时
		defer cancel()
		r, err, shared := u.group.Do(username, func() (interface{}, error) {
			return u.getUserFromDBAndSetCache(dbCtx, username)
		})
		if shared {
			log.Debugw("数据库查询被合并，共享结果", "username", username)
			metrics.RequestsMerged.WithLabelValues("get").Inc()
		}
		return r, err
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*v1.User), nil
}

// 判断是否为可重试错误（如超时、临时网络错误、数据库临时错误等）
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// 1. context 超时/取消
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	// 2. 标准库 Temporary 接口
	if e, ok := err.(interface{ Temporary() bool }); ok && e.Temporary() {
		return true
	}
	// 3. 错误字符串分析（参考 shouldRetry/isRecoverableError）
	errStr := err.Error()
	recoverableErrors := []string{
		// 超时和网络错误
		"timeout", "deadline exceeded", "connection refused", "network error",
		"connection reset", "broken pipe", "no route to host",
		// 数据库临时错误
		"database is closed", "deadlock", "1213", "40001",
		"temporary", "busy", "lock", "try again",
		// 资源暂时不可用
		"resource temporarily unavailable", "too many connections",
	}
	for _, substr := range recoverableErrors {
		if strings.Contains(errStr, substr) {
			return true
		}
	}
	return false
}
