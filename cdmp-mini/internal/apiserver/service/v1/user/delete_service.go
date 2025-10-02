package user

import (
	"context"
	"fmt"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserService) DeleteCollection(ctx context.Context, username []string, force bool, opts metav1.DeleteOptions, opt *options.Options) error {
	return nil
}

func (u *UserService) Delete(ctx context.Context, username string, force bool, opts metav1.DeleteOptions, opt *options.Options) error {

	_, err := u.checkUserExist(ctx, username, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return u.executeDelete(ctx, username, force, opts)
}

// executeDelete 执行删除业务逻辑
func (u *UserService) executeDelete(ctx context.Context, username string,
	force bool, opts metav1.DeleteOptions) error {
	var err error
	if force {
		if u.Producer == nil {
			log.Errorf("生产者转换错误")
			return errors.WithCode(code.ErrKafkaFailed, "Kafka生产者未初始化")
		}
		if u.Producer == nil {
			return fmt.Errorf("producer未初始化")
		}
		// 发送到Kafka
		err := u.Producer.SendUserDeleteMessage(ctx, username)
		if err != nil {
			log.Errorf("requestID=%s: 生产者消息发送失败 username=%s, err=%v", ctx.Value("requestID"), username, err)
			return errors.WithCode(code.ErrKafkaFailed, "kafka生产者消息发送失败")
		}
		// 记录业务成功
		log.Infow("用户删除请求已发送到Kafka", "username", username)
		return nil

	} else {
		opts = metav1.DeleteOptions{Unscoped: false}
		err = u.Store.Users().Delete(ctx, username, opts, u.Options)
	}

	if err != nil {
		//	logger.Errorw("用户删除失败", "username", username, "error", err)
		return err
	}
	//	logger.Infow("用户删除成功", "username", username)
	return nil
}

func (u *UserService) checkUserExist(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	// 先尝试无锁查询缓存（大部分请求应该在这里返回）
	user, found, err := u.tryGetFromCache(ctx, username)
	if err != nil {
		// 缓存查询错误，记录但继续流程
		log.Errorf("缓存查询异常，继续流程", "error", err.Error(), "username", username)
		// 使用 WithLabelValues 来记录错误
		metrics.CacheErrors.WithLabelValues("query_failed", "get").Inc()
	}
	// 缓存命中，直接返回
	if found {
		return user, nil
	}

	// 缓存未命中，使用singleflight保护数据库查询
	result, err, shared := u.group.Do(username, func() (interface{}, error) {
		return u.getUserFromDBAndSetCache(ctx, username, username, opts)
	})
	if shared {
		log.Infow("数据库查询被合并，共享结果", "username", username)
		metrics.RequestsMerged.WithLabelValues("get").Inc()
	}
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}
	return result.(*v1.User), nil
}
