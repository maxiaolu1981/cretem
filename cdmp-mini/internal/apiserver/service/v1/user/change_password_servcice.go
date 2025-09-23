package user

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
)

func (u *UserService) ChangePassword(ctx context.Context, user *v1.User, opt *options.Options) error {
	return nil
}
