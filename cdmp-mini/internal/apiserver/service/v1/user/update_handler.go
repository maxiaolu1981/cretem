package user

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (u *UserService) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return nil
}
