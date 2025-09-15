// store/retry/retry.go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

/*
	       死锁错误 → 重试 ✅
		   超时错误 → 重试 ✅
		   连接错误 → 重试 ✅
		   重复键错误 → 不重试 ❌（业务逻辑错误）
		  其他错误 → 根据默认策略
*/
type RetryConfig struct {
	MaxAttempts       int           //最大重试次数
	InitialBackoff    time.Duration //初始退避时间
	InitialDelay      time.Duration
	MaxBackoff        time.Duration    // 最大退避时间
	BackoffMultiplier float32          //退避倍数
	IsRetryable       func(error) bool // 重试判断函数
}

var DefaultRetryConfig = RetryConfig{
	MaxAttempts:  2,
	InitialDelay: 100 * time.Millisecond,
	IsRetryable:  DefaultIsRetryable,
}

func DefaultIsRetryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, gorm.ErrInvalidTransaction) {
		return true
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1205, 1213, 2006, 2013:
			return true
		}
	}
	return false
}

func Do(ctx context.Context, config RetryConfig, operation func() error) error {
	retryDelay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}

		err := operation()
		if err == nil {
			return nil
		}

		if !config.IsRetryable(err) || attempt == config.MaxAttempts {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return errors.New("超出最大重试次数")
}
