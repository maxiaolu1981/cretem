package user

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *Users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error {
	return u.db.Model(&v1.User{}).Where("name = ?", user.Name).Updates(user).Error
}
