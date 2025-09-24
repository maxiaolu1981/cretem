package user

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/interfaces"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/producer"

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
	data, err := u.Redis.GetKey(ctx, cacheKey)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.Infof("未进行缓存缓 key=%s", cacheKey)
			return nil, false, nil
		}
		log.Errorf("redis服务失败: key=%s, err=%v", cacheKey, err)
		return nil, false, err
	}
	var user = v1.User{}
	//查找到防穿透记录
	if data == RATE_LIMIT_PREVENTION {
		log.Infof("空值缓存命中: key=%s", cacheKey)
		user.Name = RATE_LIMIT_PREVENTION
	} else {
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			return nil, false, errors.WithCode(code.ErrDecodingFailed, "数据解码失败")
		}
		log.Infof("从缓存中查询到用户:%s: key=%s", user.Name, cacheKey)
	}
	return &user, true, nil
}

// getUserFromDBAndSetCache 带缓存的用户查询核心逻辑
func (u *UserService) getUserFromDBAndSetCache(ctx context.Context, username, cacheKey string, opts metav1.GetOptions, opt *options.Options) (*v1.User, error) {
	logger := log.L(ctx).WithValues("operation", "getUserWithCache")

	// 1. 查询数据库
	user, err := u.Store.Users().Get(ctx, username, opts, u.Options)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			metrics.DBQueries.WithLabelValues("not_found").Inc()
			// 用户不存在，缓存空值（防止缓存击穿）
			if err := u.cacheNullValue(cacheKey); err != nil {
				logger.Errorf("缓存设置失败", "error", err.Error())
			}

			log.Infof("设置用户%s缓存成功", username)
			return &v1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: RATE_LIMIT_PREVENTION, // 直接赋值
				},
			}, nil
		}
		return nil, err
	}

	//写入缓存（带随机过期时间防雪崩）
	if err := u.setUserCache(ctx, cacheKey, user); err != nil {
		logger.Warnw("写入缓存失败", "error", err.Error())
	}
	logger.Infof("为用户%s设置缓存成功", username)
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
		log.L(ctx).Errorf("缓存写入失败", "error", err.Error())
	}
	return nil
}

// cacheNullValue 缓存空值（防穿透）
func (u *UserService) cacheNullValue(cacheKey string) error {
	// 短暂缓存空值，防止穿透
	ctx, cancel := context.WithTimeout(context.Background(), u.Options.RedisOptions.Timeout)
	defer cancel()
	cacheKey = "genericapiserver:" + cacheKey
	_, err := u.Redis.SetNX(ctx, cacheKey, RATE_LIMIT_PREVENTION, 24*time.Hour) // 24小时过期
	return err
}

func (u *UserService) generateUserCacheKey(username string) string {
	return fmt.Sprintf("user:v1:%s", username)
}
