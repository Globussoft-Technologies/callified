package db

import "context"

// HasUnlimitedManualCalls resolves the entitlement from an active admin
// subscription in this organization, never from a client feature flag.
func (d *DB) HasUnlimitedManualCalls(ctx context.Context, orgID int64) (bool, error) {
	var enabled bool
	err := d.pool.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM admin_subscriptions s
		JOIN users u ON u.email = s.admin_email
		WHERE u.org_id = ? AND u.role = 'Admin' AND u.is_active = 1
		AND s.plan = 'manual' AND s.is_active = 1 AND s.expires_at > UTC_TIMESTAMP()
	)`, orgID).Scan(&enabled)
	return enabled, err
}
