package store

var client Factory

type Factory interface {
	Users() UserStore
}

func Client() Factory {
	return client
}

func SetClient(factory Factory) {
	client = factory
}
