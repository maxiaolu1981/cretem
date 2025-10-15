package user

import (
	"context"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/usercache"

	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {

	log.Debugf("service:开始处理用户%v创建请求...", user.Name)

	// 统一规整邮箱和手机号，确保后续索引和缓存命中
	user.Email = usercache.NormalizeEmail(user.Email)
	user.Phone = usercache.NormalizePhone(user.Phone)

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
	go func() {
		defer wg.Done()
		start := time.Now()
		err := u.ensureContactUniqueness(parallelCtx, user)
		contactsDuration = time.Since(start)
		contactsErr = err
		if err != nil {
			cancel()
		}
	}()

	go func() {
		defer wg.Done()
		start := time.Now()
		ruser, err := u.checkUserExist(parallelCtx, user.Name, false)
		existenceDuration = time.Since(start)
		existErr = err
		if err == nil {
			existingUser = ruser
			if ruser != nil && ruser.Name != RATE_LIMIT_PREVENTION {
				cancel()
			}
		}
	}()

	wg.Wait()

	u.recordUserCreateStep(ctx, "ensure_contacts_unique", "all", user.Name, contactsDuration, contactsErr)
	u.recordUserCreateStep(ctx, "check_user_exist", "username", user.Name, existenceDuration, existErr)

	if contactsErr != nil {
		if errors.Is(contactsErr, context.Canceled) || errors.Is(contactsErr, context.DeadlineExceeded) {
			if existingUser != nil && existingUser.Name != RATE_LIMIT_PREVENTION {
				log.Debugw("唯一性检查因并行取消提前退出", "username", user.Name)
				contactsErr = nil
			}
		}
	}

	if contactsErr != nil {
		return contactsErr
	}
	if existErr != nil {
		log.Debugf("查询用户%s checkUserExist方法返回错误, 可能是系统繁忙, 将忽略是否存在的检查, 放行该用户: %v", user.Name, existErr)
	}
	if existingUser != nil && existingUser.Name != RATE_LIMIT_PREVENTION {
		log.Debugf("用户%s已经存在,无法创建", user.Name)
		return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在")
	}

	if u.Producer == nil {
		log.Errorf("生产者转换错误")
		return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
	}
	// 发送到Kafka
	sendStart := time.Now()
	errKafka := u.Producer.SendUserCreateMessage(ctx, user)
	u.recordUserCreateStep(ctx, "kafka_send_create_user", "kafka", user.Name, time.Since(sendStart), errKafka)
	if errKafka != nil {
		log.Errorf("requestID=%v: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), user.Name, errKafka)
		return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
	}
	log.Debugw("用户创建请求已发送到Kafka", "username", user.Name)

	return nil
}
