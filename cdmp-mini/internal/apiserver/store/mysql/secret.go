package mysql

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"gorm.io/gorm"
)

type secret struct {
	db *gorm.DB
}

func newSecrets(ds *datastore) *secret {
	return &secret{
		db: ds.DB,
	}
}

func (s *secret) Create(ctx context.Context, secret *v1.Secret, opts metav1.CreateOptions) error {
	return nil
}

func (s *secret) Update(ctx context.Context, secret *v1.Secret, opts metav1.UpdateOptions) error {
	return nil
}

func (s *secret) Delete(ctx context.Context, username, secretID string, opts metav1.DeleteOptions) error {
	return nil
}
func (s *secret) DeleteCollection(ctx context.Context, username string, secretIDs []string, opts metav1.DeleteOptions) error {
	return nil
}

func (s *secret) Get(ctx context.Context, username, secretID string, opts metav1.GetOptions) (*v1.Secret, error) {
	return nil, nil
}

func (s *secret) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.SecretList, error) {
	return nil, nil
}
