/*
包摘要
该包（package core）提供了 HTTP 响应处理的核心功能，定义了统一的错误响应格式（ErrResponse）和响应写入函数（WriteResponse）。其主要作用是规范服务端与客户端之间的响应格式，特别是错误信息的标准化输出，确保前后端交互的一致性。
核心流程
WriteResponse 函数是核心，其处理流程如下：
判断是否有错误：若传入 err 不为空，则进入错误处理流程；否则直接返回成功响应。
错误解析：通过 errors.ParseCoderByErr 将错误解析为自定义的错误编码结构（errors.Coder），该结构包含业务错误码、用户可见消息和 HTTP 状态码。
错误响应构建：根据解析结果，生成 ErrResponse 结构体（包含错误码、消息和参考信息），并以对应的 HTTP 状态码返回给客户端。
成功响应处理：若没有错误，直接以 200 OK 状态码返回业务数据（data）。

*/

package core

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/nexuscore/errors" // 自定义错误处理包
	"github.com/maxiaolu1981/cretem/nexuscore/log"    // 自定义日志包
)

// ErrResponse 定义了错误发生时的返回格式
// 若不存在参考文档，Reference 字段会被省略
// swagger:model  // 用于 Swagger 文档生成，标记该结构体为 API 模型
type ErrResponse struct {
	// Code 表示业务错误码（非 HTTP 状态码）
	Code int `json:"code"`

	// Message 包含错误的详细描述，适合对外暴露（用户可理解的信息）
	Message string `json:"message"`

	// Reference 指向解决该错误的参考文档（可选，不存在时会省略）
	Reference string `json:"reference,omitempty"`
}

// WriteResponse 将错误信息或响应数据写入 HTTP 响应体
// 它通过 errors.ParseCoder 解析任意错误为 errors.Coder 接口
// errors.Coder 包含错误码、用户安全的错误消息和对应的 HTTP 状态码
func WriteResponse(c *gin.Context, err error, data interface{}) {
	if err != nil {
		// 1. 记录错误详情到日志（便于后端排查，包含完整错误堆栈）
		log.Errorf("%#+v", err)

		// 2. 将错误解析为自定义错误编码结构（包含业务码、HTTP状态码等）
		coder := errors.ParseCoderByErr(err)

		log.Debugf("core:返回的业务码%v", coder.Code())

		// 3. 构建错误响应并返回（使用错误编码中定义的 HTTP 状态码）
		c.JSON(coder.HTTPStatus(), ErrResponse{
			Code:      coder.Code(),      // 业务错误码
			Message:   coder.String(),    // 用户可见的错误消息
			Reference: coder.Reference(), // 参考文档（可选）
		})

		return
	}

	// 4. 无错误时，返回 200 OK 和业务数据
	c.JSON(http.StatusOK, data)
}

// -------------------------- 新增：WriteDeleteSuccess --------------------------
// WriteDeleteSuccess 处理 DELETE 操作成功的响应（RESTful 规范：204 No Content）
// 核心：仅返回 204 状态码，无响应体（符合 HTTP 规范）
func WriteDeleteSuccess(c *gin.Context) {
	// 设置 204 状态码（成功且无内容），不写入任何响应体
	c.Status(http.StatusNoContent)
}

// SuccessResponse 统一成功响应结构体
type SuccessResponse struct {
	Code    int         `json:"code"`    // 成功码固定为0（与错误码区分）
	Message string      `json:"message"` // 成功提示信息
	Data    interface{} `json:"data"`    // 业务数据（单资源对象）
}

// WriteSuccessResponse 写入成功响应（适用于所有成功场景，如GET单资源、创建资源等）
// 参数：
//   - c: gin上下文
//   - message: 成功提示信息（如"查询用户详情成功"）
//   - data: 业务数据（如单条用户记录）
func WriteSuccessResponse(c *gin.Context, message string, data interface{}) {
	// 成功响应HTTP状态码固定为200 OK（符合RESTful规范）
	c.JSON(http.StatusOK, SuccessResponse{
		Code:    0,       // 成功码固定为0，区别于错误码（如100004）
		Message: message, // 自定义成功提示（语义化）
		Data:    data,    // 单资源数据（如过滤后的用户对象）
	})
}
