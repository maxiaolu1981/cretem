package core

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"github.com/maxiaolu1981/cretem/nexuscore/log"
)

// WriteResponse 处理HTTP响应，确保状态码正确设置且不被覆盖
func WriteResponse(c *gin.Context, err error, data interface{}) {
	if err != nil {
		if errors.IsWithCode(err) {
			code := errors.GetCode(err)
			log.Debugf("core:code=%v", code)
			httpStatus := errors.GetHTTPStatus(err)
			message := errors.GetMessage(err)

			// 设置响应并强制终止后续处理
			c.JSON(httpStatus, gin.H{
				"code":    code,
				"message": message,
				"data":    nil,
			})
			c.AbortWithStatus(httpStatus) // 关键：强制使用当前状态码并终止
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": "服务内部错误：" + errors.GetMessage(err),
			"data":    nil,
		})
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	msg, _ := data.(string)
	// 处理成功场景
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": msg,
		"data":    data,
	})
	c.AbortWithStatus(http.StatusOK)
}

// -------------------------- 新增：WriteDeleteSuccess --------------------------
// WriteDeleteSuccess 处理 DELETE 操作成功的响应（RESTful 规范：204 No Content）
// 核心：仅返回 204 状态码，无响应体（符合 HTTP 规范）
func WriteDeleteSuccess(c *gin.Context) {
	// 设置 204 状态码（成功且无内容），不写入任何响应体
	c.Status(http.StatusNoContent)
}

func CreateSuccessResponse(c *gin.Context, message string, data interface{}) {

	if user, ok := data.(*v1.User); ok {
		c.Header("Location", fmt.Sprintf("/users/%d", user.ID))
	}
	c.JSON(http.StatusCreated, SuccessResponse{
		Code:    code.ErrSuccess, // 成功码固定为0，区别于错误码（如100004）
		Message: message,         // 自定义成功提示（语义化）
		Data:    data,            // 单资源数据（如过滤后的用户对象）
	})
}

// SuccessResponse 统一成功响应结构体
type SuccessResponse struct {
	Code    int         `json:"code"`    // 成功码固定为0（与错误码区分）
	Message string      `json:"message"` // 成功提示信息
	Data    interface{} `json:"data"`    // 业务数据（单资源对象）
}
