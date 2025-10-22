package user

import (
	"context"
	"strings"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/usercache"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
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
		Select("name").
		Where("email = ?", normalized).
		Take(user).Error
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
		Select("name").
		Where("phone = ?", normalized).
		Take(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "用户不存在")
		}
		return nil, err
	}
	return user, nil
}

// PreflightConflicts detects existing records matching username/email/phone in a single round-trip.
func (u *Users) PreflightConflicts(ctx context.Context, username, email, phone string, _ *options.Options) (map[string]*v1.User, error) {
	if u == nil || u.db == nil {
		return nil, errors.WithCode(code.ErrDatabase, "用户存储未初始化")
	}

	normalizedName := strings.TrimSpace(username)
	normalizedEmail := usercache.NormalizeEmail(email)
	normalizedPhone := usercache.NormalizePhone(phone)

	queries := make([]string, 0, 3)
	args := make([]interface{}, 0, 3)

	if normalizedName != "" {
		queries = append(queries, "SELECT 'username' AS scope, name, email, phone, status FROM user WHERE name = ?")
		args = append(args, normalizedName)
	}
	if normalizedEmail != "" {
		queries = append(queries, "SELECT 'email' AS scope, name, email, phone, status FROM user WHERE email = ?")
		args = append(args, normalizedEmail)
	}
	if normalizedPhone != "" {
		queries = append(queries, "SELECT 'phone' AS scope, name, email, phone, status FROM user WHERE phone = ?")
		args = append(args, normalizedPhone)
	}

	if len(queries) == 0 {
		return map[string]*v1.User{}, nil
	}

	sql := strings.Join(queries, " UNION ALL ")
	type conflictRow struct {
		Scope  string
		Name   string
		Email  string
		Phone  string
		Status int32
	}

	rows := make([]conflictRow, 0, len(queries))
	if err := u.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]*v1.User{}, nil
		}
		return nil, errors.WithCode(code.ErrDatabase, "预检查查询失败: %v", err)
	}

	result := make(map[string]*v1.User, len(rows))
	for _, row := range rows {
		if row.Scope == "" {
			continue
		}
		if _, exists := result[row.Scope]; exists {
			continue
		}
		result[row.Scope] = &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: row.Name},
			Email:      row.Email,
			Phone:      row.Phone,
			Status:     int(row.Status),
		}
	}
	return result, nil
}
