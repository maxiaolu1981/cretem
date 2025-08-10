// Copyright 2025 马晓璐 <15940995655@13..com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
// log:context.go
// 提供基于上下文（context.Context）的日志实例传递功能，
// 支持将日志对象（Logger）存入上下文，以及从上下文中提取日志对象，
// 便于在函数调用链中传递带特定上下文信息的日志实例，实现日志上下文的追踪与共享。

package log

import (
	"context"
)

type key int

const (
	logContextKey key = iota
)

// WithContext returns a copy of context in which the log value is set.
func (l *zapLogger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, logContextKey, l)
}

// FromContext returns the value of the log key on the ctx.
func FromContext(ctx context.Context) Logger {
	if ctx != nil {
		logger := ctx.Value(logContextKey)
		if logger != nil {
			return logger.(Logger)
		}
	}
	return WithName("Unknown-Context")
}
