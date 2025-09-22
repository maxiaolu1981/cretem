package user

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/server/bloomfilter"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions, opt *options.Options) error {
	startTime := time.Now()
	logger := log.L(ctx).WithValues(
		"service", "UserService",
		"method", "Create",
	)

	defer func() {
		// 记录业务处理时间
		duration := time.Since(startTime).Seconds()
		metrics.BusinessProcessingTime.WithLabelValues("user_create").Observe(duration)
	}()

	//检查bloom过滤器
	bloom, err := bloomfilter.GetFilter()
	if err != nil {
		metrics.RecordBloomFilterCheck("username", "error", 0)
		metrics.SetBloomFilterStatus("username", false)
	}
	if bloom != nil {
		start := time.Now()
		exists := bloom.Test("username", user.Name)
		duration := time.Since(start)
		//如果存在,需要数据库确认
		if exists {
			metrics.RecordBloomFilterCheck("username", "hit", duration)
			user, err := u.Store.Users().Get(ctx, user.Name, metav1.GetOptions{}, u.Options)
			if err == nil && user != nil {
				return errors.WithCode(code.ErrUserAlreadyExist, "用户已经存在%s", user.Name)
			}
		} else {
			metrics.RecordBloomFilterCheck("username", "miss", duration)
		}
	}

	if u.Producer == nil {
		log.Errorf("生产者转换错误")
		return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
	}
	if u.Producer == nil {
		return fmt.Errorf("producer未初始化")
	}
	// 发送到Kafka
	err = u.Producer.SendUserCreateMessage(ctx, user)
	if err != nil {
		log.Errorf("requestID=%s: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), user.Name, err)
		metrics.BusinessFailures.WithLabelValues("user_create", "kafka_send_failed").Inc()
		return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
	}
	// 记录业务成功
	metrics.BusinessSuccess.WithLabelValues("user_create").Inc()
	logger.Infow("用户创建请求已发送到Kafka", "username", user.Name)
	return nil
}
