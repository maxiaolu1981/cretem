package user

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/core"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *UserController) List(c *gin.Context) {
	var r metav1.ListOptions
	if err := c.ShouldBindQuery(&r); err != nil {
		core.WriteResponse(c, errors.WithCode(code.ErrBind, "传入的参数错误"), nil) // ErrBind - 400: 100003请求体绑定结构体失败
		return
	}
	logger := log.L(c).WithValues(
		"controller", "UserController", // 标识当前控制器
		"action", "List", // 标识当前操作
		"client_ip", c.ClientIP(), // 客户端IP
		"method", c.Request.Method, // 请求方法
		"kind", r.Kind,
		"apiVersion", r.APIVersion,
		"labelSelector", r.LabelSelector,
		"fieldSelector", r.FieldSelector,
		"timeoutSeconds", *r.TimeoutSeconds,
		"offset", *r.Offset,
		"limit", r.Limit,
		"path", c.FullPath(), // 请求路径
		"user_agent", c.Request.UserAgent(),
	)
	logger.Debugf("开始处理用户查询请求(多资源)")
	errs := u.validateListOptions(&r)
	if len(errs) > 0 {
		errDetails := make(map[string]string, len(errs))
		for _, fieldErr := range errs {
			errDetails[fieldErr.Field] = fieldErr.ErrorBody()
		}
		detailStr := fmt.Sprintf("参数错误:%+v", errDetails)
		err := errors.WrapC(nil, code.ErrInvalidParameter, "%s", detailStr)
		core.WriteResponse(c, err, nil)
		return
	}

	userList, err := u.srv.Users().List(c, r, u.options)
	if err != nil {
		errWrap := errors.WrapC(err, code.ErrInternal, "%s", errors.GetMessage(err))
		core.WriteResponse(c, errWrap, nil)
		return
	}

	var publicUser *v1.PublicUser
	var publicUsers []*v1.PublicUser

	if len(userList.Items) > 0 {
		for _, u := range userList.Items {
			publicUser = v1.ConvertToPublicUser(u)
			publicUsers = append(publicUsers, publicUser)
		}

	}
	core.WriteResponse(c, nil, publicUsers)

}
