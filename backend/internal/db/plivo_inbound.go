package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrPlivoInboundContext = errors.New("incoming call is closed or belongs to another context")

// EnsurePlivoInboundCall serializes retries for one account so concurrent
// answer callbacks cannot create duplicate logs. No schema migration is needed.
func (d *DB) EnsurePlivoInboundCall(ctx context.Context, accountID, orgID int64, callSID, phone string) error {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin inbound call: %w", err)
	}
	defer tx.Rollback()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM org_exotel_accounts WHERE id=? AND org_id=? AND provider='plivo' FOR UPDATE`, accountID, orgID).Scan(&lockedID); err != nil {
		return fmt.Errorf("lock inbound account: %w", err)
	}
	var existingOrg int64
	var provider, status, existingPhone string
	err = tx.QueryRowContext(ctx, `SELECT org_id, provider, status, COALESCE(phone,'') FROM call_logs WHERE call_sid=? AND org_id=? ORDER BY id DESC LIMIT 1`, callSID, orgID).Scan(&existingOrg, &provider, &status, &existingPhone)
	switch {
	case err == nil:
		if existingOrg != orgID || provider != "plivo-inbound" || status != "in-progress" || existingPhone != phone {
			return ErrPlivoInboundContext
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO call_logs (lead_id, campaign_id, org_id, agent_user_id, call_sid, provider, phone, status) VALUES (0,NULL,?,NULL,?,'plivo-inbound',?,'in-progress')`, orgID, callSID, phone); err != nil {
			return fmt.Errorf("save incoming call: %w", err)
		}
	default:
		return fmt.Errorf("lookup incoming call: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit incoming call: %w", err)
	}
	return nil
}

func (d *DB) UpdatePlivoInboundStatus(ctx context.Context, orgID int64, callSID, status string) error {
	_, err := d.pool.ExecContext(ctx, `UPDATE call_logs SET status=? WHERE org_id=? AND call_sid=? AND provider='plivo-inbound' AND status='in-progress'`, status, orgID, callSID)
	if err != nil {
		return fmt.Errorf("update incoming call status: %w", err)
	}
	return nil
}
