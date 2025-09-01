/*
组件	饭店比喻	职责
Factory接口	仓库总调度系统	定义仓库访问标准
mysqlFactory	智能自动化仓库	MySQL数据库实现
sqlserverFactory	传统大型仓库	SQL Server实现
redisFactory	快速冷藏库	Redis缓存实现
Client()	调度热线电话	获取当前仓库系统
SetClient()	更换仓库供应商	设置具体仓库实现
*/

package store

var client Factory

type Factory interface {
	Users() UserStore
	Secrets() SecretStore
	Polices() PolicyStore
	PolicyAudits() PolicyAuditStore
	Close() error
}

func Client() Factory {
	return client
}
func SetClient(factory Factory) {
	client = factory
}
