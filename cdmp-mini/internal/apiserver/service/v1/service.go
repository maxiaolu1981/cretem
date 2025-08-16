package v1

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

// 角色:后厨总菜单，明确告知control，，后厨能够处理哪些大类的业务
// 作用:定义业务层入口规范,让control层知道有哪些业务能力.就好像是饭店菜单上的大类,比如川菜 粤菜.
type Service interface {
	Users() UserSrv
}

// 后厨的总调度台
// 角色:是Service接口的具体实现 相当于后厨的总调度台 它不直接处理业务 而是分配任务  当需要处理用户相关任务的时候 它会创建专业团队(userService)
// 持有数据访问层store的引用,确保具体业务处理着(userService)能够拿到食材(数据)
// 通过User() 方法孵化具体的业务处理着userService,实现总调度和具体业务分离.就像后厨领班 不用亲自炒菜 但知道哪个厨师团队负责 当服务员点菜的时候 就把任务分配给不同的厨师团队(userService)
type service struct {
	store store.Factory
}

// NewService returns Service interface.
// 确保具体业务处理着(userService)能够拿到食材(数据)
func NewService(store store.Factory) Service {
	log.Info("service:厨房调度拿到了仓库的食材")
	return &service{
		store: store,
	}
}

// 分配专业厨师团队
func (s *service) Users() UserSrv {
	log.Info("service:总调度说:好的,我让用户业务团队来处理用户相关业务")
	return newUsers(s)
}
