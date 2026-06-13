package db

import "time"

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
	Notes         string `json:"notes"`
	CreatedAt     string `json:"created_at"`
	FirstName     string `json:"first_name"`
	Phone         string `json:"phone"`
}

// CreateScheduledCall inserts a new scheduled call.
func (d *DB) CreateScheduledCall(orgID, leadID, campaignID int64, scheduledAt time.Time, notes string) (int64, error) {
	res, err := d.pool.Exec(`
		INSERT INTO scheduled_calls (org_id, lead_id, campaign_id, scheduled_at, status, notes)
		VALUES (?,?,?,?,'pending',?)`,
		orgID, leadID, nullInt64(campaignID), scheduledAt, nullString(notes))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetScheduledCallsByOrg returns all scheduled calls for an org ordered by scheduled_at ASC.
// Joins leads so the UI can show Lead Name + Phone on each row (matches Python
// get_scheduled_calls_by_org which also joins leads).
func (d *DB) GetScheduledCallsByOrg(orgID int64) ([]ScheduledCall, error) {
	rows, err := d.pool.Query(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%d %H:%i:%s'),
		COALESCE(sc.status,'pending'), COALESCE(sc.notes,''),
		DATE_FORMAT(sc.created_at,'%Y-%m-%d %H:%i:%s'),
		COALESCE(l.first_name,''), COALESCE(l.phone,'')
		FROM scheduled_calls sc
		LEFT JOIN leads l ON sc.lead_id=l.id
		WHERE sc.org_id=? ORDER BY sc.scheduled_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledCalls(rows)
}

// GetPendingScheduledCalls returns pending calls due within the next
// `leadTimeSeconds` seconds. Picking rows up slightly *before* their target
// time absorbs the provider-API + telco handoff (~2–4 s for Twilio/Exotel),
// so the phone rings at the exact second the user scheduled. Pass 0 for
// strict "<= NOW()" semantics.
//
// No lead join here — the scheduler worker resolves lead details via GetLeadByID.
func (d *DB) GetPendingScheduledCalls(leadTimeSeconds int) ([]ScheduledCall, error) {
	if leadTimeSeconds < 0 {
		leadTimeSeconds = 0
	}
	rows, err := d.pool.Query(`
		SELECT sc.id, sc.org_id, sc.lead_id, COALESCE(sc.campaign_id,0),
		DATE_FORMAT(sc.scheduled_at,'%Y-%m-%d %H:%i:%s'),
		COALESCE(sc.status,'pending'), COALESCE(sc.notes,''),
		DATE_FORMAT(sc.created_at,'%Y-%m-%d %H:%i:%s'),
		'', ''
		FROM scheduled_calls sc
		WHERE sc.status='pending' AND sc.scheduled_at <= DATE_ADD(NOW(), INTERVAL ? SECOND)
		ORDER BY sc.scheduled_at ASC`, leadTimeSeconds)
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
			&sc.ScheduledAt, &sc.Status, &sc.Notes, &sc.CreatedAt,
			&sc.FirstName, &sc.Phone); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, rows.Err()
}
