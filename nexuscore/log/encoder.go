/*
// package log
// encode.go
// Copyright 2025 马晓璐 <15940995655@13..com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
// 提供Zap日志库的自定义编码功能，
// 包括时间格式化为"2006-01-02 15:04:05.000"的编码器，
// 以及将时间间隔转换为毫秒浮点数的编码器，
// 用于统一日志中时间和时长的输出格式。
*/

package log

import (
	"time"

	"go.uber.org/zap/zapcore"
)

func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

func milliSecondsDurationEncoder(d time.Duration, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendFloat64(float64(d) / float64(time.Millisecond))
}
