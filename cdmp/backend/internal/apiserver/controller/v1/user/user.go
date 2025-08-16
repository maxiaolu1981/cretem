/*
user 包定义了用户资源的控制器（UserController），作为 HTTP 接口与业务逻辑层（Service）之间的桥梁。核心职责是初始化用户控制器实例，并通过依赖注入关联业务层服务（srvv1.Service），为后续处理用户相关的 HTTP 请求（如查询、创建用户）提供基础。
核心流程
该包的核心是创建和初始化 UserController 实例，流程如下：
控制器定义：UserController 结构体持有业务层服务实例（srv），通过该实例调用用户相关的业务逻辑。
实例化控制器：NewUserController 函数接收存储层工厂（store.Factory）作为参数，先创建业务层服务实例（srvv1.NewService(store)），再将其注入 UserController，返回初始化完成的控制器实例。
依赖传递：通过存储层工厂 → 业务层服务 → 控制器的依赖链，实现各层解耦，确保控制器无需直接依赖存储层，仅通过业务层接口调用逻辑。

*/

package user

import (
	// 引入业务层服务的 v1 版本接口
	srvv1 "github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/service/v1"
	// 引入存储层工厂接口
	"github.com/maxiaolu1981/cretem/cdmp/backend/internal/apiserver/store"
)

// UserController 定义用户资源的控制器，用于处理用户相关的 HTTP 请求。
// 持有service业务层服务实例，通过调用服务层方法完成业务逻辑。
type UserController struct {
	srv srvv1.Service // 业务层服务实例，提供用户相关的业务逻辑接口
}

// NewUserController 创建并初始化 UserController 实例。
// 参数 store：存储层工厂，用于创建业务层服务实例。
// 返回：初始化完成的 UserController 指针。
func NewUserController(store store.Factory) *UserController {
	return &UserController{
		// 初始化业务层服务（注入存储层工厂），并将其关联到控制器
		srv: srvv1.NewService(store),
	}
}
