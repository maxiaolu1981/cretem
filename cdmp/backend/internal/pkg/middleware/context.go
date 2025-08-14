// Copyright (c) 2025 马晓璐
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.
/*
包摘要
该包（package middleware）提供了一个 Gin 中间件 Context，用于在请求上下文中注入通用的前缀字段（如请求 ID、用户名），方便后续日志记录或业务逻辑中快速获取这些公共信息。
核心流程
Context 中间件的执行流程如下：
从 Gin 上下文（c *gin.Context）中获取已存在的 X-Request-ID（请求唯一标识）和 username（用户名）。
将这两个值分别以日志包定义的键（log.KeyRequestID、log.KeyUsername）存入上下文，供后续处理器（中间件或业务逻辑）使用。
调用 c.Next() 传递请求给下一个处理器，完成中间件的执行。

*/

package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/nexuscore/log" // 日志包，定义了日志字段的键
)

// UsernameKey 定义在 Gin 上下文中存储用户名的键名
const UsernameKey = "username"

// Context 是一个中间件，用于向 Gin 上下文注入通用的前缀字段（如请求ID、用户名），
// 便于后续日志记录或业务逻辑统一获取这些公共信息。
func Context() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从上下文获取 X-Request-ID（由 RequestID 中间件设置），并以日志包的 KeyRequestID 为键存入上下文
		// 作用：后续日志打印时可直接从上下文获取该键，统一日志格式中的请求ID字段
		c.Set(log.KeyRequestID, c.GetString(XRequestIDKey))

		// 2. 从上下文获取用户名（可能由认证中间件等提前设置），并以日志包的 KeyUsername 为键存入上下文
		// 作用：后续日志或业务逻辑可快速获取当前操作用户，便于审计或权限校验
		c.Set(log.KeyUsername, c.GetString(UsernameKey))

		// 3. 调用下一个中间件或业务处理器
		c.Next()
	}
}
