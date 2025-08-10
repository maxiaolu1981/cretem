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

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNew 测试基础错误创建功能
func TestNew(t *testing.T) {
	t.Run("正常创建错误", func(t *testing.T) {
		msg := "测试错误"
		err := New(msg)

		// 验证错误消息
		assert.Equal(t, msg, err.Error())

		// 验证错误类型
		fundErr, ok := err.(*fundamental)
		assert.True(t, ok, "错误类型应为 fundamental")

		// 验证堆栈不为空
		assert.NotNil(t, fundErr.stack, "应包含堆栈信息")
	})
}

// TestErrorf 测试格式化错误创建功能
func TestErrorf(t *testing.T) {
	t.Run("格式化消息正确", func(t *testing.T) {
		format := "用户 %s 不存在"
		args := []interface{}{"testuser"}
		expectedMsg := fmt.Sprintf(format, args...)

		err := Errorf(format, args...)
		assert.Equal(t, expectedMsg, err.Error())

		// 验证堆栈存在
		fundErr, ok := err.(*fundamental)
		assert.True(t, ok)
		assert.NotNil(t, fundErr.stack)
	})
}

// TestWithStack 测试堆栈包装功能
func TestWithStack(t *testing.T) {
	t.Run("包装普通错误", func(t *testing.T) {
		rootErr := New("根错误")
		wrappedErr1 := WithStack(rootErr)
		wrappedErr2 := WithStack(wrappedErr1)

		// 验证类型
		stackErr2, ok := wrappedErr2.(*withStack)
		assert.True(t, ok)

		// 验证根因
		assert.Equal(t, wrappedErr1, stackErr2.Cause())

		// 验证堆栈存在
		assert.NotNil(t, stackErr2.stack)
	})

	t.Run("包装带码错误", func(t *testing.T) {
		code := 1001
		rootErr := WithCode(code, "带码根错误")
		wrappedErr := WithStack(rootErr)

		// 验证类型
		codeErr, ok := wrappedErr.(*withCode)
		assert.True(t, ok)

		// 验证错误码保留
		assert.Equal(t, code, codeErr.code)

		// 验证根因
		assert.Equal(t, rootErr, codeErr.Cause())
	})

	t.Run("包装nil错误", func(t *testing.T) {
		assert.Nil(t, WithStack(nil))
	})
}

// TestWrap 测试错误包装（带消息）功能
func TestWrap(t *testing.T) {
	t.Run("包装普通错误", func(t *testing.T) {
		rootErr := New("根错误")
		msg := "包装消息"
		wrappedErr := Wrap(rootErr, msg)

		// 验证消息
		assert.Equal(t, msg, wrappedErr.Error())

		// 验证根因链
		stackErr, ok := wrappedErr.(*withStack)
		assert.True(t, ok)

		msgErr, ok := stackErr.error.(*withMessage)
		assert.True(t, ok)
		assert.Equal(t, rootErr, msgErr.Cause())
	})

	t.Run("包装带码错误", func(t *testing.T) {
		code := 1002
		rootErr := WithCode(code, "带码根错误")
		msg := "包装消息"

		wrappedErr := Wrap(rootErr, msg)
		codeErr, ok := wrappedErr.(*withCode)
		assert.True(t, ok)

		// 验证错误码和消息
		assert.Equal(t, code, codeErr.code)
		assert.Equal(t, msg, codeErr.err.Error())
		assert.Equal(t, rootErr, codeErr.Cause())
	})

	t.Run("包装nil错误", func(t *testing.T) {
		assert.Nil(t, Wrap(nil, "任意消息"))
	})
}

// TestWrapf 测试格式化错误包装功能
func TestWrapf(t *testing.T) {
	t.Run("格式化消息正确", func(t *testing.T) {
		rootErr := New("根错误")
		format := "用户 %d 操作失败"
		uid := 123
		expectedMsg := fmt.Sprintf(format, uid)

		wrappedErr := Wrapf(rootErr, format, uid)
		assert.Equal(t, expectedMsg, wrappedErr.Error())
	})
}

// TestWithMessage 测试仅添加消息的包装功能
func TestWithMessage(t *testing.T) {
	t.Run("消息添加正确", func(t *testing.T) {
		rootErr := New("根错误")
		msg := "附加消息"

		wrappedErr := WithMessage(rootErr, msg)
		assert.Equal(t, msg, wrappedErr.Error())

		// 验证根因
		msgErr, ok := wrappedErr.(*withMessage)
		assert.True(t, ok)
		assert.Equal(t, rootErr, msgErr.Cause())
	})

	t.Run("包装nil错误", func(t *testing.T) {
		assert.Nil(t, WithMessage(nil, "消息"))
	})
}

// TestWithCode 测试带错误码的错误创建
func TestWithCode(t *testing.T) {
	t.Run("错误码和消息正确", func(t *testing.T) {
		code := 2001
		format := "错误码测试: %s"
		arg := "参数"
		expectedMsg := fmt.Sprintf(format, arg)

		err := WithCode(code, format, arg)
		codeErr, ok := err.(*withCode)
		assert.True(t, ok)

		// 验证错误码和消息
		assert.Equal(t, code, codeErr.code)
		assert.Equal(t, expectedMsg, codeErr.err.Error())
		assert.NotNil(t, codeErr.stack)
		assert.Nil(t, codeErr.cause, "根错误不应有上一级错误")
	})
}

// TestWrapC 测试为现有错误添加错误码
func TestWrapC(t *testing.T) {
	t.Run("包装现有错误", func(t *testing.T) {
		rootErr := New("根错误")
		code := 3001
		format := "包装消息: %d"
		num := 100
		expectedMsg := fmt.Sprintf(format, num)

		wrappedErr := WrapC(rootErr, code, format, num)
		codeErr, ok := wrappedErr.(*withCode)
		assert.True(t, ok)

		// 验证错误码、消息和根因
		assert.Equal(t, code, codeErr.code)
		assert.Equal(t, expectedMsg, codeErr.err.Error())
		assert.Equal(t, rootErr, codeErr.Cause())
		assert.NotNil(t, codeErr.stack)
	})

	t.Run("包装nil错误", func(t *testing.T) {
		assert.Nil(t, WrapC(nil, 100, "消息"))
	})
}

// TestCause 测试获取错误根因功能
func TestCause(t *testing.T) {
	t.Run("多层包装的根因", func(t *testing.T) {
		rootErr := New("最底层错误")
		level1 := Wrap(rootErr, "第一层包装")
		level2 := WrapC(level1, 4001, "第二层包装: %s", "参数错误")
		level3 := WithStack(level2)

		// 最终根因应为最底层错误
		assert.Equal(t, rootErr, Cause(level3))
	})

	t.Run("非包装错误的根因", func(t *testing.T) {
		plainErr := fmt.Errorf("标准库错误")
		assert.Equal(t, plainErr, Cause(plainErr))
	})

	t.Run("nil错误的根因", func(t *testing.T) {
		assert.Nil(t, Cause(nil))
	})
}

// TestFormat 测试错误格式化输出
func TestFormat(t *testing.T) {
	rootErr := New("根错误")
	wrappedErr := Wrap(rootErr, "包装消息")

	t.Run("基础错误格式化", func(t *testing.T) {
		// %s 输出消息
		assert.Equal(t, "根错误", fmt.Sprintf("%s", rootErr))

		// %v 输出消息
		assert.Equal(t, "根错误", fmt.Sprintf("%v", rootErr))

		// %+v 输出消息+堆栈（堆栈包含具体地址，这里只验证前缀）
		stackOutput := fmt.Sprintf("%+v", rootErr)
		assert.Contains(t, stackOutput, "根错误")
		// 验证堆栈包含函数调用信息（而非固定字符串）
		assert.Contains(t, stackOutput, "github.com/cretem/nexuscore/errors")
	})

	t.Run("包装错误格式化", func(t *testing.T) {
		// %+v 输出根因+当前消息
		formatOutput := fmt.Sprintf("%+v", wrappedErr)
		assert.Contains(t, formatOutput, "根错误")  // 根因消息
		assert.Contains(t, formatOutput, "包装消息") // 当前消息
		assert.Contains(t, formatOutput, "github.com/cretem/nexuscore/errors")
	})
}
