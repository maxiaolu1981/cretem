package user

import (
	"context"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/trace"
	gormutil "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *Users) List(ctx context.Context, username string, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error) {
	traceCtx, span := trace.StartSpan(ctx, "user-store", "list_users")
	if traceCtx != nil {
		ctx = traceCtx
	}
	trace.AddRequestTag(ctx, "target_user", username)

	spanStatus := "success"
	spanCode := strconv.Itoa(code.ErrSuccess)
	spanDetails := map[string]any{
		"username": username,
	}
	defer func() {
		if span != nil {
			trace.EndSpan(span, spanStatus, spanCode, spanDetails)
		}
	}()

	ret := &v1.UserList{}
	ol := gormutil.Unpointer(opts.Offset, opts.Limit)

	// 构建基础查询
	query := u.db.WithContext(ctx).Model(&v1.User{}).Where("status = 1")

	// 只有在 selector 不为 nil 时才调用方法
	if username != "" {
		query = query.Where("name = ?", username)
	}

	// 先获取总数
	if err := query.Count(&ret.TotalCount).Error; err != nil {
		spanStatus = "error"
		if c := errors.GetCode(err); c != 0 {
			spanCode = strconv.Itoa(c)
		}
		return nil, err
	}

	// 再获取分页数据
	if err := query.Offset(ol.Offset).
		Limit(ol.Limit).
		Order("id desc").
		Find(&ret.Items).Error; err != nil {
		spanStatus = "error"
		if c := errors.GetCode(err); c != 0 {
			spanCode = strconv.Itoa(c)
		}
		return nil, err
	}

	spanDetails["returned_count"] = len(ret.Items)
	return ret, nil
}

func (u *Users) ListAllUsernames(ctx context.Context) ([]string, error) {
	var usernames []string
	err := u.db.Model(&v1.User{}).
		Pluck("name", &usernames). // 直接获取字符串数组
		Error

	if err != nil {
		return nil, err
	}

	return usernames, nil
}

func (u *Users) ListAll(ctx context.Context, username string) (*v1.UserList, error) {
	ret := &v1.UserList{}
	query := u.db.Where("status = 1")
	if username != "" {
		query = query.Where("name like ?", "%"+username+"%")
	}

	d := query.Order("id desc").Find(&ret.Items)
	ret.TotalCount = int64(len(ret.Items))

	return ret, d.Error
}
