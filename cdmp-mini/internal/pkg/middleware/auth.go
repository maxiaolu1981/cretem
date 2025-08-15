package middleware

import (
	"github.com/gin-gonic/gin"
)

type AuthStrategy interface {
	AuthFunc() gin.HandlerFunc
}
