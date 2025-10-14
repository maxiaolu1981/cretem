package user

import (
	"context"
	"strings"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
	"gorm.io/gorm"
)

// GetByEmail locates a user record by email address.
func (u *Users) GetByEmail(ctx context.Context, email string, _ *options.Options) (*v1.User, error) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return nil, errors.WithCode(code.ErrInvalidParameter, "邮箱不能为空")
	}

	user := &v1.User{}
	err := u.db.WithContext(ctx).
		Where("email = ?", normalized).
		First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在")
		}
		return nil, err
	}
	return user, nil
}

// GetByPhone locates a user record by phone number.
func (u *Users) GetByPhone(ctx context.Context, phone string, _ *options.Options) (*v1.User, error) {
	normalized := strings.TrimSpace(phone)
	if normalized == "" {
		return nil, errors.WithCode(code.ErrInvalidParameter, "手机号不能为空")
	}

	user := &v1.User{}
	err := u.db.WithContext(ctx).
		Where("phone = ?", normalized).
		First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在")
		}
		return nil, err
	}
	return user, nil
}
