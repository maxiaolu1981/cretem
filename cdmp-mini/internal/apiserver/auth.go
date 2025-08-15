package apiserver

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/middleware"
	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/log"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/spf13/viper"
)

func newJWTAuth() middleware.AuthStrategy {
	ginJwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            viper.GetString("jwt.realm"),
		SigningAlgorithm: "HS256",
		Key:              []byte(viper.GetString("jwt.key")),
		Timeout:          viper.GetDuration("jwt.timeout"),
		MaxRefresh:       viper.GetDuration("jwt.max-refresh"),
		Authenticator:    authenticator(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, nil)
		},
	})
}

func authenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login loginInfo
		var err error
		if c.Request.Header.Get("Authorization") != "" {
			login, err = parseWithHeader(c)
		} else {
			login, err = parseWithBody(c)
		}
		if err != nil {
			return "", jwt.ErrFailedAuthentication
		}
		user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
		if err != nil {
			log.Errorf("查询用户信息失败:%s", err.Error())
			return "", jwt.ErrFailedAuthentication
		}
		if err := user.Compare(login.Password); err != nil {
			return "", jwt.ErrFailedAuthentication
		}
		user.LoginedAt = time.Now()
		_ = store.Client().Users().Update(c, user, metav1.UpdateOptions{})
		return user, nil
	}

}

func parseWithHeader(c *gin.Context) (loginInfo, error) {
	auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		log.Errorf("从头解析Basic失败")
		return loginInfo{}, jwt.ErrFailedAuthentication
	}
	payload, err := base64.StdEncoding.DecodeString(auth[1])
	if err != nil {
		log.Errorf("decode basic string:%s", err.Error())
		return loginInfo{}, jwt.ErrEmptyAuthHeader
	}
	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		log.Errorf("用户名密码解析失败")
		return loginInfo{}, jwt.ErrFailedAuthentication
	}
	return loginInfo{
		Username: pair[0],
		Password: pair[1],
	}, nil
}

func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("解析用户名密码错误,%s", err.Error())
		return loginInfo{}, jwt.ErrFailedAuthentication
	}
	return login, nil
}

type loginInfo struct {
	Username string `form:"username" json:"username" binding:"required,username"`
	Password string `form:"password" json:"password" binding:"required,password"`
}
