package db

import (
	"fmt"
	"strings"
	"time"
)

// ScheduledCall mirrors the scheduled_calls table.
//
// JSON keys match the Python response shape so the ScheduledCallsPage can
// render without frontend changes — in particular `scheduled_time` (the page
// reads call.scheduled_time, not scheduled_at) and the lead's first_name/phone
// joined from the leads table. Without the JOIN the page rendered rows with
// blank Lead Name / Phone cells even when data was present.
type ScheduledCall struct {
	ID            int64  `json:"id"`
	OrgID         int64  `json:"org_id"`
	LeadID        int64  `json:"lead_id"`
	CampaignID    int64  `json:"campaign_id"`
	ScheduledAt   string `json:"scheduled_time"`
	Status        string `json:"status"`
	Mode          string `json:"mode"`
	Notes         string `json:"notes"`
	ExecutiveID   int64  `json:"executive_id"`
	ExecutiveName string `json:"executive_name"`
	CreatedAt     string `json:"created_at"`
	FirstName     string `json:"first_name"`
	Phone         string `json:"phone"`
}

// EnsureScheduledCallsTable creates the scheduled_calls table if it doesn't
// exist and adds columns introduced by the Go backend on legacy schemas.
func (d *DB) EnsureScheduledCallsTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS scheduled_calls (
			id INT AUTO_INCREMENT PRIMARY KEY,
			org_id INT NOT NULL,
			campaign_id INT,
			lead_id INT NOT NULL,
			scheduled_time DATETIME NOT NULL,
			scheduled_at DATETIME DEFAULT NULL,
			status ENUM('pending','dialing','completed','failed','cancelled') DEFAULT 'pending',
			mode VARCHAR(20) DEFAULT 'ai',
			executive_id INT DEFAULT NULL,
			notes TEXT DEFAULT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_scheduled_pending (status, scheduled_at),
			INDEX idx_scheduled_mode (mode, status, scheduled_at),
			INDEX idx_scheduled_exec (executive_id),
			FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
			FOREIGN KEY (lead_id) REFERENCES leads (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return err
	}

	columns := []struct{ name, def string }{
		{"scheduled_at", "DATETIME DEFAULT NULL"},
		{"notes", "TEXT DEFAULT NULL"},
		{"mode", "VARCHAR(20) DEFAULT 'ai'"},
		{"executive_id", "INT DEFAULT NULL"},
	}
	for _, col := range columns {
		_, alterErr := d.pool.Exec(fmt.Sprintf("ALTER TABLE scheduled_calls ADD COLUMN %s %s", col.name, col.def))
		if alterErr != nil && !strings.Contains(alterErr.Error(), "Duplicate column name") {
			return alterErr
		}
	}

	// Backfill legacy rows so the scheduler doesn't skip them after the mode
	// column is added.
	_, _ = d.pool.Exec(`UPDATE scheduled_calls SET mode='ai' WHERE mode IS NULL OR mode=''`)
	return nil
}

// CreateScheduledCall inserts a new scheduled call.
func (d *DB) CreateScheduledCall(orgID, leadID, campaignID, executiveID int64, scheduledAt time.Time, notes, mode string) (int64, error) {
	if mode == "" {
		mode = "ai"
	}
	res, err := d.pool.Exec(`
		INSERT INTO scheduled_calls (org_id, lead_id, campaign_id, scheduled_at, status, mode, executive_id, notes)
		VALUES (?,?,?,?,'pending',?,?,?)`,
		orgID, leadID, nullInt64(campaignID), scheduledAt, mode, nullInt64(executiveID), nullString(notes))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetPendingScheduledCallByLead returns the newest pending scheduled call for a lead, if any.
func (d *DB) GetPendingScheduledCallByLead(leadID int64) (*ScheduledCall, error) {
	row := d.pool.QueryRow(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(sc.status,'pending'), COALESCE(sc.mode,'ai'), COALESCE(sc.notes,''),
		COALESCE(sc.executive_id,0), COALESCE(e.name,''),
		DATE_FORMAT(sc.created_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(l.first_name,''), COALESCE(l.phone,'')
		FROM scheduled_calls sc
		LEFT JOIN leads l ON sc.lead_id=l.id
		LEFT JOIN executives e ON sc.executive_id=e.id
		WHERE sc.lead_id=? AND sc.status='pending'
		ORDER BY sc.scheduled_at DESC, sc.id DESC
		LIMIT 1`, leadID)
	var sc ScheduledCall
	if err := row.Scan(&sc.ID, &sc.OrgID, &sc.LeadID, &sc.CampaignID,
		&sc.ScheduledAt, &sc.Status, &sc.Mode, &sc.Notes,
		&sc.ExecutiveID, &sc.ExecutiveName,
		&sc.CreatedAt,
		&sc.FirstName, &sc.Phone); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &sc, nil
}

// UpdateScheduledCall updates the scheduled time, notes, mode and executive of an existing call.
func (d *DB) UpdateScheduledCall(id int64, scheduledAt time.Time, notes, mode string, executiveID int64) error {
	if mode == "" {
		mode = "ai"
	}
	_, err := d.pool.Exec(`
		UPDATE scheduled_calls
		SET scheduled_at=?, notes=?, mode=?, executive_id=?
		WHERE id=?`,
		scheduledAt, nullString(notes), mode, nullInt64(executiveID), id)
	return err
}

// GetScheduledCallsByOrg returns all scheduled calls for an org ordered by scheduled_at ASC.
// Joins leads so the UI can show Lead Name + Phone on each row (matches Python
// get_scheduled_calls_by_org which also joins leads).
func (d *DB) GetScheduledCallsByOrg(orgID int64) ([]ScheduledCall, error) {
	rows, err := d.pool.Query(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(sc.status,'pending'), COALESCE(sc.mode,'ai'), COALESCE(sc.notes,''),
		COALESCE(sc.executive_id,0), COALESCE(e.name,''),
		DATE_FORMAT(sc.created_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(l.first_name,''), COALESCE(l.phone,'')
		FROM scheduled_calls sc
		LEFT JOIN leads l ON sc.lead_id=l.id
		LEFT JOIN executives e ON sc.executive_id=e.id
		WHERE sc.org_id=? ORDER BY sc.scheduled_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledCalls(rows)
}

// GetPendingScheduledCalls returns pending AI calls due within the next
// `leadTimeSeconds` seconds. Manual calls are excluded because the frontend
// handles those via reminders.
func (d *DB) GetPendingScheduledCalls(leadTimeSeconds int) ([]ScheduledCall, error) {
	if leadTimeSeconds < 0 {
		leadTimeSeconds = 0
	}
	rows, err := d.pool.Query(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(sc.status,'pending'), COALESCE(sc.mode,'ai'), COALESCE(sc.notes,''),
		COALESCE(sc.executive_id,0), '',
		DATE_FORMAT(sc.created_at,'%Y-%m-%dT%H:%i:%sZ'),
		'', ''
		FROM scheduled_calls sc
		WHERE sc.status='pending' AND sc.mode='ai'
		  AND sc.scheduled_at <= DATE_ADD(UTC_TIMESTAMP(), INTERVAL ? SECOND)
		ORDER BY sc.scheduled_at ASC`, leadTimeSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledCalls(rows)
}

// GetDueManualScheduledCalls returns pending manual calls whose scheduled time
// has arrived (or is about to arrive within the next few seconds).
func (d *DB) GetDueManualScheduledCalls(orgID int64, leadTimeSeconds int) ([]ScheduledCall, error) {
	if leadTimeSeconds < 0 {
		leadTimeSeconds = 0
	}
	rows, err := d.pool.Query(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(sc.status,'pending'), COALESCE(sc.mode,'manual'), COALESCE(sc.notes,''),
		COALESCE(sc.executive_id,0), COALESCE(e.name,''),
		DATE_FORMAT(sc.created_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(l.first_name,''), COALESCE(l.phone,'')
		FROM scheduled_calls sc
		LEFT JOIN leads l ON sc.lead_id=l.id
		LEFT JOIN executives e ON sc.executive_id=e.id
		WHERE sc.org_id=? AND sc.status='pending' AND sc.mode='manual'
		  AND sc.scheduled_at <= DATE_ADD(UTC_TIMESTAMP(), INTERVAL ? SECOND)
		ORDER BY sc.scheduled_at ASC`, orgID, leadTimeSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledCalls(rows)
}

// UpdateScheduledCallStatus sets the status column (e.g., "completed", "failed", "cancelled").
func (d *DB) UpdateScheduledCallStatus(id int64, status string) error {
	_, err := d.pool.Exec(`UPDATE scheduled_calls SET status=? WHERE id=?`, status, id)
	return err
}

// CancelScheduledCall marks a pending call as cancelled. Returns true if updated.
func (d *DB) CancelScheduledCall(orgID, id int64) (bool, error) {
	res, err := d.pool.Exec(
		`UPDATE scheduled_calls SET status='cancelled'
		 WHERE id=? AND org_id=? AND status='pending'`, id, orgID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanScheduledCalls(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]ScheduledCall, error) {
	var list []ScheduledCall
	for rows.Next() {
		var sc ScheduledCall
		if err := rows.Scan(&sc.ID, &sc.OrgID, &sc.LeadID, &sc.CampaignID,
			&sc.ScheduledAt, &sc.Status, &sc.Mode, &sc.Notes,
			&sc.ExecutiveID, &sc.ExecutiveName,
			&sc.CreatedAt,
			&sc.FirstName, &sc.Phone); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, rows.Err()
}
