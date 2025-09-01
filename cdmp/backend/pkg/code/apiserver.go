// Copyright 2025 马晓璐 <15940995655@163.com>. 保留所有权利。
// 本代码的使用受 MIT 风格许可证约束，该许可证可在 LICENSE 文件中找到。

package code

//go:generate codegen -type=int

// iam-apiserver：用户相关错误
const (
	// ErrUserNotFound - 404: 用户不存在。
	ErrUserNotFound int = iota + 110001

	// ErrUserAlreadyExist - 400: 用户已存在。
	ErrUserAlreadyExist
	//用户表有外键约束
	ErrInvalidReference
)

// iam-apiserver：密钥相关错误
const (
	// ErrReachMaxCount - 400: 密钥数量已达上限。
	ErrReachMaxCount int = iota + 110101

	// ErrSecretNotFound - 404: 密钥不存在。
	ErrSecretNotFound
)

// iam-apiserver：策略相关错误
const (
	// ErrPolicyNotFound - 404: 策略不存在。
	ErrPolicyNotFound int = iota + 110201
)
