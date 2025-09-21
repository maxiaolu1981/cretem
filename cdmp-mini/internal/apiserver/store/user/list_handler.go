package user

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	gormutil "github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/util"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/component-base/fields"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *Users) List(ctx context.Context, opts metav1.ListOptions, opt *options.Options) (*v1.UserList, error) {
	ret := &v1.UserList{}
	ol := gormutil.Unpointer(opts.Offset, opts.Limit)

	selector, _ := fields.ParseSelector(opts.FieldSelector)
	username, _ := selector.RequiresExactMatch("name")
	d := u.db.Where("name like ? and status = 1", "%"+username+"%").
		Offset(ol.Offset).
		Limit(ol.Limit).
		Order("id desc").
		Find(&ret.Items).
		Offset(-1).
		Limit(-1).
		Count(&ret.TotalCount)

	return ret, d.Error
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
