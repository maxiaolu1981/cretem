package mysql

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp/backend/pkg/code"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"

	"gorm.io/gorm"
)

type users struct {
	db *gorm.DB
}

func (u *users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {

	if opts.Kind != "User" {
		return nil, errors.WithCode(code.ErrInvalidResourceKind, "资源类型不匹配,期望:User,实际:%s", opts.Kind)
	}
	if opts.APIVersion != "v1" {
		return nil, errors.WithCode(code.ErrInvalidAPIVersion, "API版本不支持,期望:v1,实际:%s", opts.APIVersion)
	}

	user := &v1.User{}
	err := u.db.Where("name=? and status =1").First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, err.Error())
		}
		return nil, errors.WithCode(code.ErrDatabase, err.Error())
	}
	return user, nil
}

func newUsers(ds *datastore) *users {
	return &users{ds.db}
}
