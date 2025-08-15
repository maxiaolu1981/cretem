/*
包摘要
该包为基于 Gin 框架的应用提供自定义数据验证功能，主要实现了用户名和密码的合法性校验。通过向 Gin 框架默认的 validator 注册器添加自定义验证规则，使得在结构体标签中使用 "username" 和 "password" 标签即可触发相应的验证逻辑。
处理流程
定义两个自定义验证函数：validateUsername（验证用户名）和 validatePassword（验证密码）
在初始化函数中，获取 Gin 框架绑定器使用的 validator 实例
向 validator 实例注册自定义验证规则，将 "username" 标签与 validateUsername 函数关联，将 "password" 标签与 validatePassword 函数关联
注册完成后，即可在结构体字段的 binding 标签中使用 "username" 和 "password" 进行数据验证
*/
package validator

import (
	"github.com/gin-gonic/gin/binding"                                   // Gin框架的绑定器，用于获取验证器实例
	"github.com/go-playground/validator/v10"                             // 数据验证库
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/validation" // 项目中的验证工具包
)

// validateUsername 检查给定的用户名是否合法
// 参数 fl 提供了访问被验证字段的方法
func validateUsername(fl validator.FieldLevel) bool {
	// 获取字段的字符串值（用户名）
	username := fl.Field().String()
	// 调用项目验证工具检查用户名是否符合规范
	// 如果返回的错误列表长度大于0，说明验证失败
	if errs := validation.IsQualifiedName(username); len(errs) > 0 {
		return false
	}

	return true
}

// validatePassword 检查给定的密码是否合法
// 参数 fl 提供了访问被验证字段的方法
func validatePassword(fl validator.FieldLevel) bool {
	// 获取字段的字符串值（密码）
	password := fl.Field().String()
	// 调用项目验证工具检查密码是否有效
	// 如果返回错误，说明验证失败
	if err := validation.IsValidPassword(password); err != nil {
		return false
	}

	return true
}

// 包初始化函数，在包被导入时自动执行
func init() {
	// 将Gin的验证器引擎转换为validator.Validate类型
	// 这是为了使用validator的注册自定义验证函数功能
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		// 注册"username"验证规则，关联到validateUsername函数
		_ = v.RegisterValidation("username", validateUsername)
		// 注册"password"验证规则，关联到validatePassword函数
		_ = v.RegisterValidation("password", validatePassword)
	}
}
