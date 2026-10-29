package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// SaveReceptionistCallDetails edits the per-call contact and reuses a CRM lead
// within this organization. Name edits update the matched CRM contact too.
func (d *DB) SaveReceptionistCallDetails(ctx context.Context, orgID, transcriptID int64, first, last, phone, interest, status string) (int64, error) {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var leadID int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(ct.lead_id,0) FROM call_transcripts ct LEFT JOIN leads l ON l.id=ct.lead_id WHERE ct.id=? AND ct.org_id=? AND (ct.direction='inbound' OR l.source='Inbound Call') FOR UPDATE`, transcriptID, orgID).Scan(&leadID)
	if err != nil {
		return 0, err
	}
	if phone != "" {
		digits := strings.TrimPrefix(phone, "+")
		domestic := digits
		if len(digits) == 12 && strings.HasPrefix(digits, "91") {
			domestic = digits[2:]
		}
		international := digits
		if len(digits) == 10 {
			international = "91" + digits
		}
		var matched int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM leads WHERE org_id=? AND REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(phone,'+',''),' ',''),'-',''),'(',''),')','') IN (?,?) ORDER BY (id=?) DESC,id LIMIT 1 FOR UPDATE`, orgID, domestic, international, leadID).Scan(&matched)
		if err == nil {
			leadID = matched
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		} else {
			result, insertErr := tx.ExecContext(ctx, `INSERT INTO leads (org_id,first_name,last_name,phone,source,interest,company,executive_id) VALUES (?,?,?,?,'Inbound Call',?,'',NULL)`, orgID, first, last, phone, nullString(interest))
			if insertErr != nil {
				return 0, insertErr
			}
			leadID, err = result.LastInsertId()
			if err != nil {
				return 0, err
			}
		}
	}
	if leadID != 0 {
		// Only names change: preserve the CRM phone, ownership, source and other fields.
		if _, err = tx.ExecContext(ctx, `UPDATE leads SET first_name=?,last_name=? WHERE id=? AND org_id=?`, first, last, leadID, orgID); err != nil {
			return 0, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE call_transcripts SET inbound_first_name=?,inbound_last_name=?,inbound_phone=?,inbound_interest=?,inbound_status=?,lead_id=? WHERE id=? AND org_id=?`, nullString(first), nullString(last), nullString(phone), nullString(interest), nullString(status), nullInt64(leadID), transcriptID, orgID)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return leadID, nil
}
