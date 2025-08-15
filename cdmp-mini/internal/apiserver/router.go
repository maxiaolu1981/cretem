package apiserver

import "github.com/gin-gonic/gin"

func initRouter(g *gin.Engine) {
	installMiddleware(g)
	installContorller(g)

}

func installMiddleware(g *gin.Engine) {

}

func installContorller(g *gin.Engine) {
   jwtStrategy,_ := new 
}
