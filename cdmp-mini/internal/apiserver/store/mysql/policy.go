package mysql

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"gorm.io/gorm"
)

type policy struct {
	db *gorm.DB
}

func newPolices(ds *datastore) *policy {
	return &policy{
		db: ds.DB,
	}
}

func (p *policy) Create(ctx context.Context, policy *v1.Policy, opts metav1.CreateOptions) error {
	return nil
}

func (p *policy) Update(ctx context.Context, policy *v1.Policy, opts metav1.UpdateOptions) error {
	return nil
}

func (p *policy) Delete(ctx context.Context, username string, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (p *policy) DeleteCollection(ctx context.Context, username string, names []string, opts metav1.DeleteOptions) error {
	return nil
}

func (p *policy) Get(ctx context.Context, username string, name string, opts metav1.GetOptions) (*v1.Policy, error) {
	return nil, nil
}

func (p *policy) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.PolicyList, error) {
	return nil, nil
}

// DeleteByUser deletes policies by username.
func (p *policy) DeleteByUser(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	db := p.db  // 使用局部变量
	if opts.Unscoped {
		db = db.Unscoped()  // 只修改局部变量
	}

	return db.Where("username = ?", username).Delete(&v1.Policy{}).Error
}
