// Copyright 2025 马晓璐 <15940995655@163.com>. 保留所有权利。
// 本代码的使用受 MIT 风格许可证约束，该许可证可在 LICENSE 文件中找到。

package code

//go:generate codegen -type=int
//go:generate codegen -type=int -doc -output ../../../docs/guide/zh-CN/api/error_code_generated.md

// 通用错误：基础错误类型
// 错误码必须以 1xxxxx 开头
const (
	// ErrSuccess - 200: ok。
	ErrSuccess int = iota + 100001

	// ErrUnknown - 500: 服务器内部错误。
	ErrUnknown

	// ErrBind - 400: 后端接收 HTTP 请求并解析请求体(如 JSON、表单数据等) 发生错误。
	ErrBind

	// ErrValidation - 400: 验证失败。
	ErrValidation

	// ErrTokenInvalid - 401: 令牌无效。
	ErrTokenInvalid

	// ErrPageNotFound - 404: 页面不存在。
	ErrPageNotFound
)

// 通用错误：数据库相关错误
const (
	// ErrDatabase - 500: 数据库错误。
	ErrDatabase int = iota + 100101
)

// 通用错误：授权与认证相关错误
const (
	// ErrEncrypt - 401: 加密用户密码时发生错误。
	ErrEncrypt int = iota + 100201

	// ErrSignatureInvalid - 401: 签名无效。
	ErrSignatureInvalid

	// ErrExpired - 401: 令牌已过期。
	ErrExpired

	// ErrInvalidAuthHeader - 401: 无效的授权头。
	ErrInvalidAuthHeader

	// ErrMissingHeader - 401: "Authorization" 头为空。
	ErrMissingHeader

	// ErrPasswordIncorrect - 401: 密码不正确。
	ErrPasswordIncorrect

	// ErrPermissionDenied - 403: 权限不足。
	ErrPermissionDenied
)

// 通用错误：编码/解码相关错误
const (
	// ErrEncodingFailed - 500: 由于数据错误导致编码失败。
	ErrEncodingFailed int = iota + 100301

	// ErrDecodingFailed - 500: 由于数据错误导致解码失败。
	ErrDecodingFailed

	// ErrInvalidJSON - 500: 数据不是有效的 JSON 格式。
	ErrInvalidJSON

	// ErrEncodingJSON - 500: JSON 数据编码失败。
	ErrEncodingJSON

	// ErrDecodingJSON - 500: JSON 数据解码失败。
	ErrDecodingJSON

	// ErrInvalidYaml - 500: 数据不是有效的 Yaml 格式。
	ErrInvalidYaml

	// ErrEncodingYaml - 500: Yaml 数据编码失败。
	ErrEncodingYaml

	// ErrDecodingYaml - 500: Yaml 数据解码失败。
	ErrDecodingYaml
)
