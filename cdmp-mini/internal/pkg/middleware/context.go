package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

const UsernameKey = "username"

func Context() gin.HandlerFunc {

	return func(c *gin.Context) {
		// username := "提取到的用户名"
		// c.Set("username", username)
		fmt.Printf("存入上下文的username: [%s]\n", c.GetString(UsernameKey)) // 观察是否为空
		c.Set(log.KeyRequestID, c.GetString(XRequestIDKey))
		c.Set(log.KeyUsername, c.GetString(UsernameKey))
		c.Next()
	}
}
