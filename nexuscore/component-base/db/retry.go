// store/retry/retry.go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	IsRetryable  func(error) bool
}

var DefaultRetryConfig = RetryConfig{
	MaxRetries:   2,
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

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}

		err := operation()
		if err == nil {
			return nil
		}

		if !config.IsRetryable(err) || attempt == config.MaxRetries {
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
