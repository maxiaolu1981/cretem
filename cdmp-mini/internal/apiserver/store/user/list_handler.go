package user

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *Users) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	ret := &v1.UserList{}

	// 移除分页和条件限制，直接查询所有记录的name字段
	d := u.db.Model(&v1.User{}). // 使用Model明确指定查询模型
					Select("name").   // 只选择name字段
					Find(&ret.Items). // 查询所有记录
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
