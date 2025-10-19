package user

import (
	"context"
	"strconv"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/trace"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

func (u *Users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions, opt *options.Options) error {
	storeCtx, span := trace.StartSpan(ctx, "user-store", "update_user")
	if storeCtx != nil {
		ctx = storeCtx
	}
	trace.AddRequestTag(ctx, "target_user", user.Name)

	spanStatus := "success"
	spanCode := strconv.Itoa(code.ErrSuccess)
	spanDetails := map[string]any{
		"username": user.Name,
	}
	defer func() {
		if span != nil {
			trace.EndSpan(span, spanStatus, spanCode, spanDetails)
		}
	}()

	if err := u.db.WithContext(ctx).Model(&v1.User{}).Where("name = ?", user.Name).Updates(user).Error; err != nil {
		spanStatus = "error"
		if c := errors.GetCode(err); c != 0 {
			spanCode = strconv.Itoa(c)
		}
		return err
	}
	return nil
}
