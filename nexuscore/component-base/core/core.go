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
	// 自定义日志包
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
		// 将错误解析为自定义错误编码结构（包含业务码、HTTP状态码等）
		coder := errors.ParseCoderByErr(err)
		//构建错误响应并返回（使用错误编码中定义的 HTTP 状态码）
		c.JSON(coder.HTTPStatus(), ErrResponse{
			Code:      coder.Code(),      // 业务错误码
			Message:   coder.String(),    // 用户可见的错误消息
			Reference: coder.Reference(), // 参考文档（可选）
		})
		return
	}
	// ✅ 根据请求路径和方法决定状态码
	statusCode := determineStatusCode(c.Request)
	c.JSON(statusCode, data)

}

// ✅ 明确的状态码判断逻辑
// ✅ 明确的状态码判断逻辑
func determineStatusCode(req *http.Request) int {
	path := req.URL.Path
	method := req.Method

	// 登录认证操作 - 返回200
	if path == "/login" && method == "POST" {
		return http.StatusOK
	}

	// 删除操作 - 返回204
	if method == "DELETE" {
		return http.StatusNoContent
	}

	// 真正的资源创建操作 - 返回201
	if method == "POST" && isResourceCreation(path) {
		return http.StatusCreated
	}

	// 其他所有情况 - 返回200
	return http.StatusOK
}

// ✅ 明确定义哪些是资源创建端点
func isResourceCreation(path string) bool {
	resourceCreationPaths := []string{
		"/v1/users",
	}

	for _, p := range resourceCreationPaths {
		if path == p {
			return true
		}
	}
	return false
}
