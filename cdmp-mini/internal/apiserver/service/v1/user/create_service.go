package user

import (
	"context"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/trace"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/usercache"

	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/auth"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) (err error) {
	ctx, span := trace.StartSpan(ctx, "user-service", "create")
	trace.AddRequestTag(ctx, "username", user.Name)
	businessCode := strconv.Itoa(code.ErrSuccess)
	spanStatus := "success"
	defer func() {
		if err != nil {
			spanStatus = "error"
			if c := errors.GetCode(err); c != 0 {
				businessCode = strconv.Itoa(c)
			} else {
				businessCode = strconv.Itoa(code.ErrUnknown)
			}
		}
		trace.EndSpan(span, spanStatus, businessCode, map[string]interface{}{
			"username": user.Name,
		})
	}()

	// 统一规整邮箱和手机号，确保后续索引和缓存命中
	user.Email = usercache.NormalizeEmail(user.Email)
	user.Phone = usercache.NormalizePhone(user.Phone)

	// 对密码进行加密，避免在控制层重复执行
	passwordStart := time.Now()
	if user.Password != "" {
		hashed, hashErr := auth.Encrypt(user.Password)
		u.recordUserCreateStep(ctx, "encrypt_password", "password", user.Name, time.Since(passwordStart), hashErr)
		if hashErr != nil {
			log.Errorf("用户密码加密失败: username=%s, err=%v", user.Name, hashErr)
			return errors.WithCode(code.ErrEncrypt, "用户密码加密失败")
		}
		user.Password = hashed
	} else {
		u.recordUserCreateStep(ctx, "encrypt_password", "password", user.Name, time.Since(passwordStart), nil)
	}

	u.ensureContactCacheReady()

	parallelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg                sync.WaitGroup
		contactsErr       error
		existingUser      *v1.User
		existErr          error
		contactsDuration  time.Duration
		existenceDuration time.Duration
	)

	wg.Add(2)
	go func(ctx context.Context) {
		defer wg.Done()
		spanCtx, contactSpan := trace.StartSpan(ctx, "user-service", "ensure_contacts_unique")
		start := time.Now()
		err := u.ensureContactUniqueness(spanCtx, user)
		contactsDuration = time.Since(start)
		contactsErr = err
		status := "success"
		codeStr := strconv.Itoa(code.ErrSuccess)
		if err != nil {
			cancel()
			status = "error"
			if c := errors.GetCode(err); c != 0 {
				codeStr = strconv.Itoa(c)
			} else {
				codeStr = strconv.Itoa(code.ErrUnknown)
			}
		}
		trace.EndSpan(contactSpan, status, codeStr, map[string]interface{}{
			"username":    user.Name,
			"duration_ms": contactsDuration.Milliseconds(),
		})
	}(parallelCtx)

	go func(ctx context.Context) {
		defer wg.Done()
		spanCtx, checkSpan := trace.StartSpan(ctx, "user-service", "check_user_exist")
		start := time.Now()
		ruser, err := u.checkUserExist(spanCtx, user.Name, false)
		existenceDuration = time.Since(start)
		existErr = err
		if err == nil {
			existingUser = ruser
			if ruser != nil && ruser.Name != RATE_LIMIT_PREVENTION {
				cancel()
			}
		}
		status := "success"
		codeStr := strconv.Itoa(code.ErrSuccess)
		if err != nil {
			status = "error"
			if c := errors.GetCode(err); c != 0 {
				codeStr = strconv.Itoa(c)
			} else {
				codeStr = strconv.Itoa(code.ErrUnknown)
			}
		}
		trace.EndSpan(checkSpan, status, codeStr, map[string]interface{}{
			"username":    user.Name,
			"duration_ms": existenceDuration.Milliseconds(),
		})
	}(parallelCtx)

	wg.Wait()

	u.recordUserCreateStep(ctx, "ensure_contacts_unique", "all", user.Name, contactsDuration, contactsErr)
	u.recordUserCreateStep(ctx, "check_user_exist", "username", user.Name, existenceDuration, existErr)

	if contactsErr != nil {
		if errors.Is(contactsErr, context.Canceled) || errors.Is(contactsErr, context.DeadlineExceeded) {
			if existingUser != nil && existingUser.Name != RATE_LIMIT_PREVENTION {
				log.Warnf("唯一性检查因并行取消提前退出", "username", user.Name)
				contactsErr = nil
			}
		}
	}

	if contactsErr != nil {
		err = contactsErr
		return err
	}
	if existErr != nil {
		log.Warnf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查, 放行该用户: %v", user.Name, existErr)
	}
	if existingUser != nil && existingUser.Name != RATE_LIMIT_PREVENTION {
		log.Warnf("用户%s已经存在,无法创建", user.Name)
		err = errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在")
		return err
	}

	if u.Producer == nil {
		log.Errorf("生产者转换错误")
		err = errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
		return err
	}
	// 发送到Kafka
	sendStart := time.Now()
	sendCtx, sendSpan := trace.StartSpan(ctx, "user-service", "producer_send_create")
	trace.AddRequestTag(sendCtx, "username", user.Name)
	errKafka := u.Producer.SendUserCreateMessage(sendCtx, user)
	u.recordUserCreateStep(ctx, "kafka_send_create_user", "kafka", user.Name, time.Since(sendStart), errKafka)
	sendStatus := "success"
	sendCode := strconv.Itoa(code.ErrSuccess)
	if errKafka != nil {
		log.Errorf("requestID=%v: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), user.Name, errKafka)
		sendStatus = "error"
		if c := errors.GetCode(errKafka); c != 0 {
			sendCode = strconv.Itoa(c)
		} else {
			sendCode = strconv.Itoa(code.ErrUnknown)
		}
		err = errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
	}
	trace.EndSpan(sendSpan, sendStatus, sendCode, map[string]interface{}{
		"username": user.Name,
	})
	if errKafka != nil {
		return err
	}

	return nil
}
