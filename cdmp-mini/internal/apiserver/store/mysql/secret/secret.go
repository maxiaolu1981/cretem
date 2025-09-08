package secret

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"gorm.io/gorm"
)



type Secret struct {
	Db *gorm.DB
}

func (s *Secret) Create(ctx context.Context, secret *v1.Secret, opts metav1.CreateOptions) error {
	return nil
}

func (s *Secret) Update(ctx context.Context, secret *v1.Secret, opts metav1.UpdateOptions) error {
	return nil
}

func (s *Secret) Delete(ctx context.Context, username, secretID string, opts metav1.DeleteOptions) error {
	return nil
}
func (s *Secret) DeleteCollection(ctx context.Context, username string, secretIDs []string, opts metav1.DeleteOptions) error {
	return nil
}

func (s *Secret) Get(ctx context.Context, username, secretID string, opts metav1.GetOptions) (*v1.Secret, error) {
	return nil, nil
}

func (s *Secret) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.SecretList, error) {
	return nil, nil
}
