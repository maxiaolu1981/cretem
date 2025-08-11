/*
store 包定义了用户数据（User）的存储层接口（UserStore），规范了用户数据的增删改查（CRUD）操作。该接口是存储层与上层服务（如 service 层）的交互契约，具体实现可由 MySQL、Redis 等不同存储引擎提供，实现了 “接口与实现分离”，便于上层模块解耦和存储引擎的替换。
核心流程
该包的核心是定义 UserStore 接口，规定用户数据操作的标准方法，流程如下：
接口定义：通过 UserStore 接口声明用户数据的基本操作（创建、更新、删除、查询等），每个方法明确输入参数（上下文、用户数据、操作选项）和返回值（结果或错误）。
参数规范：使用 context.Context 传递上下文（用于超时控制、日志追踪）；v1.User 为用户数据模型；metav1.CreateOptions 等为通用操作选项（如是否忽略钩子函数）。
上下层交互：service 层依赖 UserStore 接口调用数据操作，无需关心底层是 MySQL 还是其他存储，只需调用接口方法即可完成数据操作。
*/

package store

import (
	"context" // 标准库：提供上下文管理（超时、取消信号等）

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"           // 自定义库：用户数据模型定义
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1" // 自定义库：通用操作选项（创建、更新等配置）
)

// UserStore 定义了用户数据的存储层接口，规范了用户数据的CRUD操作。
type UserStore interface {
	// Create 在存储中创建一个新用户。
	// ctx：上下文（用于传递超时、日志等）；
	// user：待创建的用户数据；
	// opts：创建操作的选项（如是否触发钩子函数）。
	Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error

	// Update 更新存储中的用户数据。
	// ctx：上下文；
	// user：包含更新内容的用户数据；
	// opts：更新操作的选项（如乐观锁版本控制）。
	Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error

	// Delete 从存储中删除指定用户名的用户。
	// ctx：上下文；
	// username：待删除用户的用户名；
	// opts：删除操作的选项（如是否级联删除关联数据）。
	Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error

	// DeleteCollection 批量删除存储中多个用户名对应的用户。
	// ctx：上下文；
	// usernames：待删除用户的用户名列表；
	// opts：批量删除操作的选项。
	DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error

	// Get 从存储中查询指定用户名的用户详情。
	// ctx：上下文；
	// username：要查询的用户名；
	// opts：查询操作的选项（如是否忽略缓存）。
	// 返回：查询到的用户数据或错误。
	Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error)

	// List 从存储中查询用户列表（支持分页、过滤等）。
	// ctx：上下文；
	// opts：列表查询选项（如分页偏移量、每页条数、排序字段）。
	// 返回：包含用户列表和总条数的结构体或错误。
	List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error)
}
