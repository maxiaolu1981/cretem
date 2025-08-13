/*
store 包是 存储层的核心入口，通过 Factory 接口定义存储层的 “能力契约”，并提供全局访问点 Client()。核心职责：
存储层解耦：通过 Factory 接口抽象存储实现（如 MySQL、Redis 等），业务层只需依赖接口，无需关心具体存储类型；
多资源存储聚合：Factory 聚合 UserStore/SecretStore 等子资源存储接口，实现 “一站式” 存储访问；
Mock 支持：通过 mockgen 自动生成 Mock 实现（mock_store.go），便于单元测试时替换真实存储。
二、核心流程
代码围绕 存储层的抽象与访问 设计，关键流程：
1. 接口定义（Factory）
定义存储层的总入口接口，要求实现类必须提供：
子资源存储的访问方法（Users()/Secrets() 等）；
存储关闭方法（Close()）。
2. 全局访问点（Client()/SetClient()）
Client()：返回全局唯一的存储工厂实例，业务层通过它访问存储能力；
SetClient()：初始化 / 替换全局存储工厂（通常在程序启动时，注入真实存储或 Mock 实现）。
3. Mock 生成（go:generate）
通过 mockgen 自动生成 Mock 实现，命令：
bash
mockgen -self_package=xxx -destination mock_store.go -package store xxx Factory,UserStore...

生成的 mock_store.go 可用于单元测试，替换真实存储，避免依赖数据库等外部服务。

*/

package store

// 注意：实际项目中需补充依赖，此处省略

//go:generate mockgen -self_package=github.com/marmotedu/iam/internal/apiserver/store -destination mock_store.go -package store github.com/marmotedu/iam/internal/apiserver/store Factory,UserStore,SecretStore,PolicyStore,PolicyAuditStore

// 全局存储工厂实例，业务层通过 Client() 访问
var client Factory

// Factory 是存储层的总入口接口，定义了获取各子资源存储的方法。
// 作用：
// 1. 聚合所有子资源存储（User/Secret/Policy 等），实现“一站式”访问；
// 2. 抽象存储实现，业务层只需依赖接口，无需关心 MySQL/Redis 等具体存储；
// 3. 支持 Mock 替换（通过 mockgen 生成 mock_store.go）。
type Factory interface {
	// Users 返回用户资源的存储操作接口
	Users() UserStore
	// Secrets 返回密钥资源的存储操作接口
	Secrets() SecretStore
	// Policies 返回策略资源的存储操作接口
	Policies() PolicyStore
	// PolicyAudits 返回策略审计资源的存储操作接口
	PolicyAudits() PolicyAuditStore
	// Close 关闭存储连接（如数据库连接池）
	Close() error
}

// Client 返回全局唯一的存储工厂实例，供业务层访问存储能力。
// 典型用法：
// userStore := store.Client().Users()
func Client() Factory {
	return client
}

// SetClient 初始化/替换全局存储工厂实例，通常在程序启动时调用。
// 示例：
// store.SetClient(mysqlStoreFactory{}) // 注入真实存储
// store.SetClient(mock_store.Factory{}) // 注入 Mock 存储（单元测试用）
func SetClient(factory Factory) {
	client = factory
}
