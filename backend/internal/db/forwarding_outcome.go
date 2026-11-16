package db

import (
	"context"
	"fmt"
)

const forwardingOutcomeSchema = `CREATE TABLE IF NOT EXISTS incoming_forwarding_outcomes (org_id BIGINT NOT NULL, call_sid VARCHAR(160) NOT NULL, outcome VARCHAR(20) NOT NULL, bleg_uuid VARCHAR(80) NOT NULL DEFAULT '', action_received BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, PRIMARY KEY(org_id,call_sid)) ENGINE=InnoDB`

// SetForwardingOutcome is independent of call_logs.status (the incoming leg).
// Answer evidence wins; otherwise a final action wins over provisional events.
func (d *DB) SetForwardingOutcome(ctx context.Context, orgID int64, callSID, outcome, bleg string, action bool) error {
	switch outcome {
	case "answered", "busy", "no_answer", "failed", "cancelled", "unknown":
	default:
		return fmt.Errorf("invalid forwarding outcome")
	}
	_, err := d.pool.ExecContext(ctx, `INSERT INTO incoming_forwarding_outcomes (org_id,call_sid,outcome,bleg_uuid,action_received)
		VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE
		bleg_uuid=IF(bleg_uuid='',VALUES(bleg_uuid),bleg_uuid),
		outcome=CASE WHEN VALUES(outcome)='answered' OR outcome='answered' THEN 'answered'
		WHEN action_received=TRUE THEN outcome
		ELSE VALUES(outcome) END,
		action_received=GREATEST(action_received,VALUES(action_received))`, orgID, callSID, outcome, bleg, action)
	if err != nil {
		return fmt.Errorf("save forwarding outcome: %w", err)
	}
	return nil
}
