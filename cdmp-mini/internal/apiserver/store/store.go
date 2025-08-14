package store

// 定义工厂接口 Factor接口,定义Users()资源
type Factory interface {
	Users() UserStore
}

// 定义工厂client
var client Factory

// 定义获取工厂方法Client
func Client() Factory {
	return client
}

// 定义修改工厂SetClient
func SetClient(factory Factory) {
	client = factory
}
