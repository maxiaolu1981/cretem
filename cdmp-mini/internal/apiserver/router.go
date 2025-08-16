package apiserver

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/control/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/auth"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func initRouter(g *gin.Engine) {
	installMiddleware(g)
	//绑定用户控制器
	installContorller(g)

}

func installMiddleware(g *gin.Engine) {

}

func installContorller(g *gin.Engine) *gin.Engine {
	jwtStrategy, _ := newJWTAuth().(auth.JWTStrategy)
	g.POST("/login", jwtStrategy.LoginHandler)
	g.POST("logout", jwtStrategy.LogoutHandler)
	g.POST("/refresh", jwtStrategy.RefreshHandler)

	auto := newAutoAuth()

	g.NoRoute(auto.AuthFunc(), func(c *gin.Context) {
		core.WriteResponse(c, errors.WithCode(code.ErrPageNotFound, "page not found"), nil)
	})
	storeIns, _ := mysql.GetMySQLFactoryOr(nil)
	v1 := g.Group("/v1")
	{
		userv1 := v1.Group("/users")
		{
			//找到厨。。最终的 UserController 持有这个厨师团队的引用（srv 字段），后续可以通过 u.srv.Users().List() 让厨师处理具体业务。
			userController := user.NewUserController(storeIns)
			log.Info("服务员通过厨房总调度拿到了厨师团队的引用,后续可以通过u.srv.Users()让厨师团队处理相关的业务")
			userv1.GET("", userController.List)

		}
		v1.Use(auto.AuthFunc())

	}
	return g
}
