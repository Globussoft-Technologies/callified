package db

import (
	"fmt"
	"time"
)

type UserExportCall struct {
	ID                                                                                                                 int64
	CallSID, CallerName, CallerEmail, LeadName, Phone, CampaignName, Provider, Status, CalledAt, Summary, RecordingURL string
	DurationS                                                                                                          float64
}

// GetUserExportCalls includes call attempts even when no recording exists.
// Admin details cover the organization; caller identity remains explicit.
func (d *DB) GetUserExportCalls(orgID, userID int64, from, to time.Time, campaignID, limit, offset int64) ([]UserExportCall, error) {
	args := []any{orgID, userID, userID, userID}
	dateScope := ""
	if !from.IsZero() {
		dateScope += " AND cl.created_at>=?"
		args = append(args, from)
	}
	if !to.IsZero() {
		dateScope += " AND cl.created_at<=?"
		args = append(args, to.Add(24*time.Hour-time.Second))
	}
	if campaignID > 0 {
		dateScope += " AND cl.campaign_id=?"
		args = append(args, campaignID)
	}
	args = append(args, limit, offset)
	rows, err := d.pool.Query(fmt.Sprintf(`SELECT cl.id,COALESCE(cl.call_sid,''),
 COALESCE(NULLIF(caller.full_name,''),caller.email,'Unknown'),COALESCE(caller.email,''),
 COALESCE(NULLIF(TRIM(CONCAT(COALESCE(l.first_name,''),' ',COALESCE(l.last_name,''))),''),cl.phone,''),
 COALESCE(cl.phone,''),COALESCE(c.name,'No campaign'),COALESCE(cl.provider,''),COALESCE(cl.status,''),
 DATE_FORMAT(cl.created_at,'%%Y-%%m-%%d %%H:%%i:%%s'),COALESCE(ct.call_duration_s,0),
 COALESCE(NULLIF(cd.edited_summary,''),NULLIF(cd.ai_summary,''),
 (SELECT cr.summary FROM call_reviews cr WHERE cr.transcript_id=ct.id ORDER BY cr.id DESC LIMIT 1),''),
 COALESCE(NULLIF(ct.recording_url,''),cl.recording_url,'')
 FROM call_logs cl
 LEFT JOIN leads l ON l.id=cl.lead_id AND l.org_id=cl.org_id
 LEFT JOIN campaigns c ON c.id=cl.campaign_id AND c.org_id=cl.org_id
 LEFT JOIN users caller ON caller.org_id=cl.org_id AND caller.id=COALESCE(NULLIF(cl.agent_user_id,0),
 (SELECT aa.user_id FROM agent_activities aa WHERE aa.org_id=cl.org_id AND aa.activity_type='call'
 AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.call_sid')) COLLATE utf8mb4_unicode_ci=cl.call_sid COLLATE utf8mb4_unicode_ci
 ORDER BY aa.id LIMIT 1))
 LEFT JOIN call_transcripts ct ON ct.id=(SELECT t.id FROM call_transcripts t WHERE t.org_id=cl.org_id
 AND t.call_sid<>'' AND t.call_sid COLLATE utf8mb4_unicode_ci=cl.call_sid COLLATE utf8mb4_unicode_ci ORDER BY t.id DESC LIMIT 1)
 LEFT JOIN call_dispositions cd ON cd.id=(SELECT d.id FROM call_dispositions d WHERE d.org_id=cl.org_id
 AND (d.call_log_id=cl.id OR d.transcript_id=ct.id) ORDER BY d.id DESC LIMIT 1)
 WHERE cl.org_id=? AND (cl.agent_user_id=? OR caller.id=?
 OR EXISTS (SELECT 1 FROM users owner WHERE owner.id=? AND owner.org_id=cl.org_id AND owner.role='Admin'))
 %s ORDER BY cl.created_at DESC,cl.id DESC LIMIT ? OFFSET ?`, dateScope), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []UserExportCall
	for rows.Next() {
		var c UserExportCall
		if err := rows.Scan(&c.ID, &c.CallSID, &c.CallerName, &c.CallerEmail, &c.LeadName, &c.Phone, &c.CampaignName, &c.Provider, &c.Status, &c.CalledAt, &c.DurationS, &c.Summary, &c.RecordingURL); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}
