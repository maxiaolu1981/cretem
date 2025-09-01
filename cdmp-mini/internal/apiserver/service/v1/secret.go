package v1

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type SecretSrv interface {
	Create(ctx context.Context, secret *v1.Secret, opts metav1.CreateOptions) error
	Update(ctx context.Context, secret *v1.Secret, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username, secretID string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username string, secretIDs []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username, secretID string, opts metav1.GetOptions) (*v1.Secret, error)
	List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.SecretList, error)
}

var _ SecretSrv = &secretService{}

type secretService struct {
	store store.Factory
}

func newSecrets(s *service) *secretService {
	return &secretService{store: s.store}
}

func (s *secretService) Create(ctx context.Context, user *v1.Secret, opts metav1.CreateOptions) error {
	return nil
}
func (s *secretService) Update(ctx context.Context, user *v1.Secret, opts metav1.UpdateOptions) error {
	return nil
}
func (s *secretService) Delete(ctx context.Context, username, secretID string, opts metav1.DeleteOptions) error {
	return nil
}
func (s *secretService) DeleteCollection(ctx context.Context, username string, secretID []string, opts metav1.DeleteOptions) error {
	return nil
}
func (s *secretService) Get(ctx context.Context, username, secretID string, opts metav1.GetOptions) (*v1.Secret, error) {
	return nil, nil
}
func (s *secretService) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.SecretList, error) {
	return nil, nil
}
