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
	"errors"
	"fmt"
	"sync"
	"testing"
)

// TestDefaultCoder 测试defaultCoder的所有方法
func TestDefaultCoder(t *testing.T) {
	tests := []struct {
		name     string
		coder    defaultCoder
		wantCode int
		wantHTTP int
		wantExt  string
		wantRef  string
	}{
		{
			name: "正常情况",
			coder: defaultCoder{
				C:    100,
				Http: 400,
				Ext:  "测试错误",
				Ref:  "http://test.com",
			},
			wantCode: 100,
			wantHTTP: 400,
			wantExt:  "测试错误",
			wantRef:  "http://test.com",
		},
		{
			name: "HTTP为0时默认返回500",
			coder: defaultCoder{
				C:    101,
				Http: 0,
				Ext:  "测试错误2",
				Ref:  "http://test.com/2",
			},
			wantCode: 101,
			wantHTTP: 500, // 预期默认值
			wantExt:  "测试错误2",
			wantRef:  "http://test.com/2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.coder.Code(); got != tt.wantCode {
				t.Errorf("Code() = %v, want %v", got, tt.wantCode)
			}
			if got := tt.coder.HTTPStatus(); got != tt.wantHTTP {
				t.Errorf("HTTPStatus() = %v, want %v", got, tt.wantHTTP)
			}
			if got := tt.coder.String(); got != tt.wantExt {
				t.Errorf("String() = %v, want %v", got, tt.wantExt)
			}
			if got := tt.coder.Reference(); got != tt.wantRef {
				t.Errorf("Reference() = %v, want %v", got, tt.wantRef)
			}
		})
	}
}

// TestRegister 测试Register函数
func TestRegister(t *testing.T) {
	// 重置全局状态
	withLock(func() {
		_codes = make(map[int]Coder)
		_codes[_unknownCode.Code()] = _unknownCode // 重新注册未知错误码
	})

	tests := []struct {
		name      string
		coder     Coder
		wantPanic bool
	}{
		{
			name: "注册新错误码",
			coder: defaultCoder{
				C:    200,
				Http: 400,
				Ext:  "参数错误",
				Ref:  "http://test.com/param",
			},
			wantPanic: false,
		},
		{
			name: "注册编码为0的错误码（应panic）",
			coder: defaultCoder{
				C:    0,
				Http: 400,
				Ext:  "无效错误",
				Ref:  "http://test.com/invalid",
			},
			wantPanic: true,
		},
		{
			name: "覆盖已存在的错误码",
			coder: defaultCoder{
				C:    200, // 与第一个测试用例相同的编码
				Http: 400,
				Ext:  "参数错误（更新）",
				Ref:  "http://test.com/param/update",
			},
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Error("预期会panic，但没有发生")
					}
				}()
			}

			Register(tt.coder)

			// 验证注册结果（如果没有panic）
			if !tt.wantPanic {
				withLock(func() {
					if _, ok := _codes[tt.coder.Code()]; !ok {
						t.Errorf("错误码 %d 未被注册", tt.coder.Code())
					}

					// 对于覆盖测试，验证是否更新
					if tt.name == "覆盖已存在的错误码" {
						if _codes[tt.coder.Code()].String() != tt.coder.String() {
							t.Error("错误码未被正确覆盖")
						}
					}
				})
			}
		})
	}

}

// TestMustRegister 测试MustRegister函数
func TestMustRegister(t *testing.T) {
	// 重置全局状态
	withLock(func() {
		_codes = make(map[int]Coder)
		_codes[_unknownCode.Code()] = _unknownCode // 重新注册未知错误码
	})

	testCoder := defaultCoder{
		C:    300,
		Http: 401,
		Ext:  "认证失败",
		Ref:  "http://test.com/auth",
	}

	// 测试正常注册
	MustRegister(testCoder)
	withLock(func() {
		if _, ok := _codes[testCoder.Code()]; !ok {
			t.Errorf("MustRegister 未能注册错误码 %d", testCoder.Code())
		}
	})

	// 测试重复注册（应panic）
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister 对于重复注册未panic")
		}
	}()
	MustRegister(testCoder)
}

// TestParseCode 测试ParseCode函数,没有嵌套错误链
func TestParseCode(t *testing.T) {
	// 重置并准备测试数据
	withLock(func() {
		_codes = make(map[int]Coder)
		_codes[_unknownCode.Code()] = _unknownCode // 重新注册未知错误码
		_codes[400] = defaultCoder{
			C:    400,
			Http: 404,
			Ext:  "资源未找到",
			Ref:  "http://test.com/notfound",
		}
	})

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "err为nil",
			err:      nil,
			wantCode: 0, // 期望返回nil
		},
		{
			name: "有效的withCode错误",
			err: &withCode{
				code: 400,
			},
			wantCode: 400,
		},
		{
			name: "未注册的错误码",
			err: &withCode{
				code: 999, // 未注册的错误码
			},
			wantCode: _unknownCode.Code(), // 期望返回未知错误码
		},
		{
			name:     "非withCode类型的错误",
			err:      errors.New("普通错误"),
			wantCode: _unknownCode.Code(), // 期望返回未知错误码
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coder := ParseCoderByErr(tt.err)
			if tt.err == nil {
				if coder != nil {
					t.Error("对于nil错误，ParseCode应返回nil")
				}
				return
			}

			if coder.Code() != tt.wantCode {
				t.Errorf("ParseCode() 返回的错误码 = %v, 期望 = %v", coder.Code(), tt.wantCode)
			}
		})
	}
}

// TestIsCode 测试IsCode函数
func TestIsCode(t *testing.T) {
	// 创建嵌套错误链
	innerErr := &withCode{code: 500}
	middleErr := &withCode{code: 501, cause: innerErr}
	outerErr := &withCode{code: 502, cause: middleErr}
	nonWithCodeErr := errors.New("普通错误")

	tests := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{
			name: "错误码匹配顶层错误",
			err:  outerErr,
			code: 502,
			want: true,
		},
		{
			name: "错误码匹配中间层错误",
			err:  outerErr,
			code: 501,
			want: true,
		},
		{
			name: "错误码匹配内层错误",
			err:  outerErr,
			code: 500,
			want: true,
		},
		{
			name: "错误码不匹配任何层级",
			err:  outerErr,
			code: 503,
			want: false,
		},
		{
			name: "非withCode类型错误",
			err:  nonWithCodeErr,
			code: 500,
			want: false,
		},
		{
			name: "nil错误",
			err:  nil,
			code: 500,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCode(tt.err, tt.code); got != tt.want {
				t.Errorf("IsCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTruncateString 测试truncateString函数
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{
			name: "字符串长度小于max",
			s:    "测试字符串",
			max:  10,
			want: "测试字符串",
		},
		{
			name: "字符串长度等于max",
			s:    "1234567890",
			max:  10,
			want: "1234567890",
		},
		{
			name: "字符串长度大于max",
			s:    "这是一个很长的测试字符串，需要被截断",
			max:  10,
			want: "这是一个很长的测试字...",
		},
		{
			name: "max为0",
			s:    "任何字符串",
			max:  0,
			want: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateString(tt.s, tt.max); got != tt.want {
				t.Errorf("truncateString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConcurrentAccess 测试并发访问的安全性
func TestConcurrentAccess(t *testing.T) {
	// 重置全局状态
	withLock(func() {
		_codes = make(map[int]Coder)
		_codes[_unknownCode.Code()] = _unknownCode
	})

	const goroutines = 100
	const codesPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 启动多个goroutine并发注册错误码
	for i := 0; i < goroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < codesPerGoroutine; j++ {
				code := base*1000 + j + 10000 // 确保编码唯一
				coder := defaultCoder{
					C:    code,
					Http: 400,
					Ext:  fmt.Sprintf("测试错误 %d", code),
					Ref:  fmt.Sprintf("http://test.com/%d", code),
				}
				Register(coder)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有错误码都被正确注册
	withLock(func() {
		expectedCount := 1 + goroutines*codesPerGoroutine // 1个未知错误码
		if len(_codes) != expectedCount {
			t.Errorf("并发注册后错误码数量不符: 实际 %d, 期望 %d", len(_codes), expectedCount)
		}
	})
}
