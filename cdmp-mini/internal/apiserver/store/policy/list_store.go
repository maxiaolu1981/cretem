package policy

import (
	"context"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
)

func (p *Policy) List(ctx context.Context, username string, opts metav1.ListOptions) (*v1.PolicyList, error) {
	// ret := &v1.PolicyList{}
	// ol := gormutil.Unpointer(opts.Offset, opts.Limit)

	// if username != "" {
	// 	p.db = p.db.Where("username = ?", username)
	// }

	// selector, _ := fields.ParseSelector(opts.FieldSelector)
	// name, _ := selector.RequiresExactMatch("name")

	// d := p.db.Where("name like ?", "%"+name+"%").
	// 	Offset(ol.Offset).
	// 	Limit(ol.Limit).
	// 	Order("id desc").
	// 	Find(&ret.Items).
	// 	Offset(-1).
	// 	Limit(-1).
	// 	Count(&ret.TotalCount)

	return nil, nil
}
