package v1

import (
	"context"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/store"
	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

type PolicySrv interface {
	Create(ctx context.Context, policy *v1.Policy, opts metav1.CreateOptions) error
	Update(ctx context.Context, policy *v1.Policy, opts metav1.UpdateOptions) error
	Delete(ctx context.Context, username string, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, username string, names []string, opts metav1.DeleteOptions) error
	Get(ctx context.Context, username string, name string, opts metav1.GetOptions) (*v1.Policy, error)
	List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.PolicyList, error)
}

var _ PolicySrv = &policservice{}

type policservice struct {
	store store.Factory
}

func newPolicies(s *service) *policservice {
	return &policservice{store: s.store}
}

func (p *policservice) Create(ctx context.Context, policy *v1.Policy, opts metav1.CreateOptions) error {
	return nil
}
func (p *policservice) Update(ctx context.Context, policy *v1.Policy, opts metav1.UpdateOptions) error {
	return nil
}
func (p *policservice) Delete(ctx context.Context, username string, name string, opts metav1.DeleteOptions) error {
	return nil
}
func (p *policservice) DeleteCollection(ctx context.Context, username string, names []string, opts metav1.DeleteOptions) error
func (p *policservice) Get(ctx context.Context, username string, name string, opts metav1.GetOptions) (*v1.Policy, error)
func (p *policservice) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.PolicyList, error)
