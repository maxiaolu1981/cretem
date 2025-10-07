package user

import (
	"context"
	"fmt"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *Users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error {
	// 只更新 password 字段
	result := u.db.Model(&v1.User{}).Where("id = ?", user.ID).UpdateColumn("password", user.Password)
	if result.RowsAffected == 0 {
		return fmt.Errorf("更新失败: 用户ID %d 不存在", user.ID)
	}
	return result.Error
}
