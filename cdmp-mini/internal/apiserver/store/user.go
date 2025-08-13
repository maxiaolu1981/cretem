package store

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type UserStore interface {
	Get(ctx context.Context, username string, opts metav1.GetOptions)(*v1.User,error)
}
