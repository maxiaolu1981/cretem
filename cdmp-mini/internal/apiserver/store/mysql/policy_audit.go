package mysql

import (
	"context"

	"gorm.io/gorm"
)

type policy_audit struct {
	db *gorm.DB
}

func newPolicyAudits(ds *datastore) *policy_audit {
	return &policy_audit{
		db: ds.db,
	}
}

func (p *policy_audit) ClearOutdated(ctx context.Context, maxReserveDays int) (int64, error) {
	return 0, nil
}
