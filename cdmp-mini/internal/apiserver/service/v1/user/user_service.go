/*
聚焦分析 checkUserExist 新参数在各业务流程里的用法，我先拆成两步：1) 解释 checkUserExist 的内部机制与 forceRefresh 的作用；2) 对照 create/update/delete/list/changePassword 等调用点说明为何选择不同的布尔值。

checkUserExist 的原理
缓存优先 + 可选回源：入口在 user_service.go 的 checkUserExist(ctx, username, forceRefresh)。它先通过 tryGetFromCache 查询 Redis（见 get_service.go），命中真实用户就直接返回；命中的是我们自己写入的负缓存 RATE_LIMIT_PREVENTION 时，会根据 forceRefresh 判断是否继续。
forceRefresh=false：直接把负缓存返回，认为“这个用户不存在”。
forceRefresh=true：即便命中了负缓存，也会日志提示“命中负缓存, 强制回源校验”，然后继续往数据库查一遍。
防止击穿的负缓存：getUserFromDBAndSetCache 查库找不到用户时，会调用 cacheNullValue，通过新改动后的 Redis.SetNX 写入带统一前缀的空值，短期内阻挡重复查询。
数据库兜底：缓存未命中或强制回源时，使用 util.RetryWithBackoff + singleflight 查询数据库，再把结果写回缓存。这段逻辑保证并发下只打一次 DB，同时对临时错误做重试。
不同业务场景如何传 forceRefresh
场景	代码位置	参数	理由
创建	create_service.go 第 15 行附近 checkUserExist(ctx, user.Name, false)	false	创建时我们只需要知道“目前是否有人占用这个用户名”。如果缓存里已经有负缓存，说明之前查库确认过不存在，可以放心创建；命中真实用户则阻止重复创建。
更新	update_service.go 第 14 行 checkUserExist(ctx, user.Name, true)	true	更新必须保证用户真实存在。即便缓存说不存在（可能是老的负缓存），也要强制回源 DB，避免误报“用户不存在”导致 404。
删除 / 批量删除	delete_service.go 第 17、31 行	true	删除尤其是强制删时，需要跨缓存验证，防止负缓存阻挡真实存在的用户被删除。
改密码	change_password_servcice.go 第 15 行	true	密码修改只针对现有用户，必须强制回源确认，不让负缓存阻塞。
查询（List/Get 前置过滤）	list_service.go 第 33 行等	true	列表在进入存储层前也要确认用户存在；否则负缓存会让查询直接中断，实际库里的用户就查不到了。Get 本身通过缓存->singleflight 也是强制最终一致。
总结来说，写操作里区分“是否需要强一致性”：

创造性动作（Create）允许负缓存阻挡，避免多余回源；
读取/变更/删除类操作为了强一致，统一传 true，强制绕过负缓存；
配合 checkUserExist 新增的 forceRefresh 逻辑，就能既保留负缓存的抗击穿效果，又避免它影响后续真实存在的用户操作。

GPT-5-Codex (Preview) • 1x
*/
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
	result, err := util.RetryWithBackoff(u.Options.RedisOptions.MaxRetries, isRetryableError, func() (interface{}, error) {
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
