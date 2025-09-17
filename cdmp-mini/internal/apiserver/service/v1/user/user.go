package user

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/storage"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"golang.org/x/sync/singleflight"
)

type UserService struct {
	Store       interfaces.Factory
	Redis       *storage.RedisCluster
	Options     *options.Options
	BloomFilter *bloom.BloomFilter
	BloomMutex  *sync.RWMutex
	Producer    producer.MessageProducer
	group       singleflight.Group
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

// 业务方法：检查用户名是否可能存在
func (us *UserService) UsernameMightExist(username string) bool {
	// 使用字符串专用的便捷方法
	return us.BloomFilter.TestString(username)
}

// getFromCache 从Redis获取缓存数据
func (u *UserService) getFromCache(ctx context.Context, cacheKey string) (*v1.User, error) {
	data, err := u.Redis.GetKey(ctx, cacheKey)
	if err != nil {
		return nil, errors.WithCode(code.ErrUnknown, "查询缓存失败")
	}
	// 检查是否是空值标记
	if data == "" {
		return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在")
	}

	// 反序列化用户数据
	var user v1.User
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		return nil, errors.WithCode(code.ErrDecodingFailed, "数据解码失败")
	}

	return &user, nil
}

// getUserWithCache 带缓存的用户查询核心逻辑
func (u *UserService) getUserWithCache(ctx context.Context, username, cacheKey string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	logger := log.L(ctx).WithValues("operation", "getUserWithCache")

	// 1. 查询数据库
	user, err := u.Store.Users().Get(ctx, username, opts, u.Options)
	if err != nil {
		logger.Debugw("数据库查询失败", "username", username, "error", err.Error())
		return nil, err
	}

	//1.处理用户不存在的情况（防缓存穿透）
	if user == nil {
		// 缓存空值，短暂过期时间
		u.cacheNullValue(ctx, cacheKey)
		return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在%s", username)
	}

	//2.写入缓存（带随机过期时间防雪崩）
	if err := u.setUserCache(ctx, cacheKey, user); err != nil {
		logger.Warnw("写入缓存失败", "error", err.Error())
		// 不返回错误，继续返回数据
	}

	return user, nil
}

// setUserCache 设置用户缓存
func (u *UserService) setUserCache(ctx context.Context, cacheKey string, user *v1.User) error {
	defer func() {
		if r := recover(); r != nil {
			log.L(ctx).Errorw("缓存设置发生panic", "recover", r)
		}
	}()

	data, err := json.Marshal(user)
	if err != nil {
		return errors.Wrap(err, "用户数据序列化失败")
	}

	// 基础过期时间 + 随机时间防雪崩
	baseExpire := 1 * time.Hour
	randomExpire := time.Duration(rand.Intn(300)) * time.Second
	expireTime := baseExpire + randomExpire

	if err := u.Redis.SetKey(ctx, cacheKey, string(data), expireTime); err != nil {
		log.L(ctx).Warnw("缓存写入失败", "error", err.Error())
	}
	return nil
}

// cacheNullValue 缓存空值（防穿透）
func (u *UserService) cacheNullValue(ctx context.Context, cacheKey string) {
	// 短暂缓存空值，防止穿透
	expireTime := 5 * time.Minute
	u.Redis.SetKey(ctx, cacheKey, "null", expireTime)
}

func (u *UserService) generateUserCacheKey(username string) string {
	return fmt.Sprintf("user:v1:%s", username)
}
