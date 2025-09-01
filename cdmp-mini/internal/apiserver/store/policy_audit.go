package store

import "context"

// PolicyAuditStore defines the policy_audit storage interface.
type PolicyAuditStore interface {
	ClearOutdated(ctx context.Context, maxReserveDays int) (int64, error)
}
