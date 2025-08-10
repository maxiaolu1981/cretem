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

// Package code 提供业务错误码的定义、注册与管理功能
//
// 包核心功能：
// 1. 定义标准化错误码结构（ErrCode），关联业务错误码、HTTP状态码、错误信息及参考链接
// 2. 实现errors.Coder接口，确保错误码可被全局错误处理机制识别
// 3. 提供错误码注册函数（register），校验并注册业务错误码，保证唯一性和合法性
//
// 架构设计：
// - 核心实体：ErrCode结构体，封装错误的核心属性（业务码、HTTP状态码等）
// - 接口适配：通过实现errors.Coder接口，使自定义错误码兼容上层错误处理逻辑
// - 注册机制：通过register函数对错误码进行合法性校验（如HTTP状态码白名单），并注册到全局管理器
// - 扩展性：预留多参考信息（refs）支持，便于未来扩展错误码的附加信息维度
//
// 使用场景：
// 用于系统中所有业务错误的标准化定义，确保错误处理、日志记录和前端展示的一致性
package code

import (
	"net/http"

	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/novalagung/gubrak"
)

// 编译期接口实现验证：确保*ErrCode类型实现了errors.Coder接口
// 若未完整实现接口方法，编译时会报错，提前暴露接口实现问题
var _ errors.Coder = &ErrCode{}

// ErrCode 自定义错误码结构，实现errors.Coder接口
// 用于封装业务错误码、对应的HTTP状态码、错误信息及参考链接
type ErrCode struct {
	C    int    // 业务错误码（如10001、20002等）
	HTTP int    // 对应的HTTP状态码（如400、404、500等）
	Ext  string // 错误描述信息（面向开发者或用户的提示）
	Ref  string // 错误参考信息（如文档链接、解决方案等）
}

// Code 返回业务错误码
// 实现errors.Coder接口的Code()方法
func (coder ErrCode) Code() int {
	return coder.C
}

// HTTPStatus 返回对应的HTTP状态码
// 若未指定HTTP状态码（值为0），默认返回500（服务器内部错误）
// 实现errors.Coder接口的HTTPStatus()方法
func (coder ErrCode) HTTPStatus() int {
	if coder.HTTP == 0 {
		return http.StatusInternalServerError
	}
	return coder.HTTP
}

// String 返回错误描述信息
// 实现errors.Coder接口的String()方法（通常用于错误信息展示）
func (coder ErrCode) String() string {
	return coder.Ext
}

// Reference 返回错误参考信息（如文档链接）
// 实现errors.Coder接口的Reference()方法
func (coder ErrCode) Reference() string {
	return coder.Ref
}

// register 注册业务错误码
// 参数说明：
//   - code: 业务错误码（自定义整数，需唯一）
//   - httpStatus: 对应的HTTP状态码（必须是预定义的合法值）
//   - message: 错误描述信息
//   - refs: 错误参考信息（可变参数，目前仅取第一个值，预留扩展）
func register(code int, httpStatus int, message string, refs ...string) {
	// 验证HTTP状态码是否在允许的范围内
	// 仅支持200（成功）、400（参数错误）、401（未授权）、403（禁止访问）、404（资源不存在）、500（服务器错误）
	allowedHTTPStatus := []int{200, 400, 401, 403, 404, 500}
	found, _ := gubrak.Includes(allowedHTTPStatus, httpStatus)
	if !found {
		panic("http编码必须定义在`200,400,401,403,404,500` 之间")
	}

	// 处理参考信息：目前仅使用第一个传入的参考值
	var reference string
	if len(refs) > 0 {
		reference = refs[0]
	}

	// 创建错误码实例
	coder := ErrCode{
		C:    code,
		HTTP: httpStatus,
		Ext:  message,
		Ref:  reference,
	}

	// 注册错误码到全局管理器
	// 若错误码已存在或不符合规范，MustRegister会触发panic
	errors.MustRegister(coder)
}

// 初始化业务编码
func init() {
	register(ErrUserNotFound, 404, "用户不存在")
	register(ErrUserAlreadyExist, 400, "用户已存在")
	register(ErrReachMaxCount, 400, "密钥数量已达上限")
	register(ErrSecretNotFound, 404, "密钥不存在")
	register(ErrPolicyNotFound, 404, "策略不存在")
	register(ErrSuccess, 200, "成功")
	register(ErrUnknown, 500, "服务器内部错误")
	register(ErrBind, 400, "后端接收 HTTP 请求并解析请求体(如 JSON、表单数据等) 发生错误")
	register(ErrValidation, 400, "验证失败")
	register(ErrTokenInvalid, 401, "令牌无效")
	register(ErrPageNotFound, 404, "页面不存在")
	register(ErrDatabase, 500, "数据库错误")
	register(ErrEncrypt, 401, "加密用户密码时发生错误")
	register(ErrSignatureInvalid, 401, "签名无效")
	register(ErrExpired, 401, "令牌已过期")
	register(ErrInvalidAuthHeader, 401, "无效的授权头")
	register(ErrMissingHeader, 401, "Authorization头为空")
	register(ErrPasswordIncorrect, 401, "密码无效", "http:mxl.com")
	register(ErrPermissionDenied, 403, "权限")
	register(ErrEncodingFailed, 500, "因数据错误导致编码失败")
	register(ErrDecodingFailed, 500, "因数据错误导致解码失败")
	register(ErrInvalidJSON, 500, "数据不是有效的JSON格式")
	register(ErrEncodingJSON, 500, "JSON数据编码失败")
	register(ErrDecodingJSON, 500, "JSON数据解码失败")
	register(ErrInvalidYaml, 500, "数据不是有效的Yaml格式")
	register(ErrEncodingYaml, 500, "Yaml数据编码失败")
	register(ErrDecodingYaml, 500, "Yaml数据解码失败")
}
