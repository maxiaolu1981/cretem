package v1

import "github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"

// 角色:后厨总菜单，明确告知control，，后厨能够处理哪些大类的业务
// 作用:定义业务层入口规范,让control层知道有哪些业务能力.就好像是饭店菜单上的大类,比如川菜 粤菜.
type Service interface {
	Users() UserSrv
	Secrets() SecretSrv
	Policies() PolicySrv
}

/*
// 后厨管理系统的工作：// 后厨的总调度台
// 1. 管理所有食材仓库（Store Factory）
// 2. 生成各个菜系的菜单（UserSrv, SecretSrv等）
// 3. 为每个菜单配备对应的厨师和食材
*/
type service struct {
	store store.Factory
}

func NewService(store store.Factory) Service {
	return &service{
		store: store,
	}
}

func (s *service) Users() UserSrv {
	return newUsers(s)
}
func (s *service) Secrets() SecretSrv {
	return newSecrets(s)
}

func (s *service) Policies() PolicySrv {
	return newPolicies(s)
}

// 持有数据访问层store的引用,确保具体业务处理着(userService)能够拿到食材(数据)
// 通过User() 方法孵化具体的业务处理着userService,实现总调度和具体业务分离.就像后厨领班 不用亲自炒菜 但知道哪个厨师团队负责 当服务员点菜的时候 就把任务分配给不同的厨师团队(userService)

// NewService returns Service interface.
// 确保具体业务处理着(userService)能够拿到食材(数据)

// 分配专业厨师团队
