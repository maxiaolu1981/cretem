package apiserver

import (
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/control/v1/user"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store/mysql"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware/auth"

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
	// log.Error("测试错误.............")
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
			userController := user.NewUserController(storeIns)
			userv1.GET("", userController.List)
		}
		v1.Use(auto.AuthFunc())

	}
	return g
}
