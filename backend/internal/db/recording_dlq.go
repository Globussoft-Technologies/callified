package db

// RecordingDLQEntry mirrors the recording_dlq table.
type RecordingDLQEntry struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	LeadID     int64  `json:"lead_id"`
	CampaignID int64  `json:"campaign_id"`
	CallSid    string `json:"call_sid"`
	StreamSid  string `json:"stream_sid"`
	ObjectKey  string `json:"object_key"`
	Provider   string `json:"provider"`
	Attempts   int    `json:"attempts"`
	LastError  string `json:"last_error"`
	CreatedAt  string `json:"created_at"`
}

// EnqueueRecordingDLQ stores a failed cloud upload for later retry.
func (d *DB) EnqueueRecordingDLQ(orgID, leadID, campaignID int64, callSid, streamSid, objectKey, provider, lastError string) error {
	_, err := d.pool.Exec(
		`INSERT INTO recording_dlq (org_id, lead_id, campaign_id, call_sid, stream_sid, object_key, provider, last_error, attempts)
		 VALUES (?,?,?,?,?,?,?,?,1)`,
		orgID, nullInt64(leadID), nullInt64(campaignID), nullString(callSid), nullString(streamSid),
		objectKey, provider, nullString(lastError))
	return err
}

// IncrementRecordingDLQAttempts increments the retry counter for a recording DLQ entry.
func (d *DB) IncrementRecordingDLQAttempts(id int64) error {
	_, err := d.pool.Exec(`UPDATE recording_dlq SET attempts = attempts + 1, updated_at = NOW() WHERE id=?`, id)
	return err
}

// DeleteRecordingDLQ removes a recording DLQ entry after successful retry.
func (d *DB) DeleteRecordingDLQ(id int64) error {
	_, err := d.pool.Exec(`DELETE FROM recording_dlq WHERE id=?`, id)
	return err
}

// GetPendingRecordingDLQ returns pending recording upload failures ordered oldest first.
func (d *DB) GetPendingRecordingDLQ(limit int) ([]RecordingDLQEntry, error) {
	rows, err := d.pool.Query(`
		SELECT id, org_id, COALESCE(lead_id,0), COALESCE(campaign_id,0), COALESCE(call_sid,''),
		       COALESCE(stream_sid,''), object_key, provider, attempts, COALESCE(last_error,''),
		       DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM recording_dlq ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RecordingDLQEntry
	for rows.Next() {
		var e RecordingDLQEntry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.LeadID, &e.CampaignID, &e.CallSid, &e.StreamSid,
			&e.ObjectKey, &e.Provider, &e.Attempts, &e.LastError, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
