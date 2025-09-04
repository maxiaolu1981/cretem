package core

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

// WriteResponse 高频调用安全版：无反射，通过工具函数解析错误
func WriteResponse(c *gin.Context, err error, data interface{}) {
	// 1. 处理错误场景
	if err != nil {
		log.Infof("WriteResponse错误处理：")
		log.Infof("  错误类型: %T", err)
		log.Infof("  IsWithCode: %v", errors.IsWithCode(err))
		log.Infof("  GetCode: %d", errors.GetCode(err))
		log.Infof("  GetHTTPStatus原始值: %d", errors.GetHTTPStatus(err)) // 关键

		// 用 errors 包工具函数判断是否为 withCode 错误（无反射）
		if errors.IsWithCode(err) {
			// 提取业务码、HTTP状态码、错误消息（均为直接访问，无性能损耗）
			code := errors.GetCode(err)
			httpStatus := errors.GetHTTPStatus(err)
			message := errors.GetMessage(err)

			// 生成 RESTful 响应
			c.Status(httpStatus)
			c.JSON(httpStatus, gin.H{
				"code":    code,
				"message": message,
				"data":    nil,
			})
			return
		}

		// 非 withCode 错误（默认 500）
		c.Status(http.StatusInternalServerError)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001, // 默认服务端错误码
			"message": "服务内部错误：" + errors.GetMessage(err),
			"data":    nil,
		})
		return
	}

	// 2. 处理成功场景（非 DELETE 操作）
	c.Status(http.StatusOK)
	c.JSON(http.StatusOK, gin.H{
		"code":    0, // 成功业务码
		"message": "操作成功",
		"data":    data,
	})
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
