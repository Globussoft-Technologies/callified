package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AgentActivityType categorizes a single agent action.
type AgentActivityType string

const (
	ActivityCall         AgentActivityType = "call"
	ActivityStatusUpdate AgentActivityType = "status_update"
	ActivityNote         AgentActivityType = "note"
	ActivityBreak        AgentActivityType = "break"
)

// AgentActivity mirrors the agent_activities table.
type AgentActivity struct {
	ID           int64             `json:"id"`
	UserID       int64             `json:"user_id"`
	OrgID        int64             `json:"org_id"`
	CampaignID   int64             `json:"campaign_id"`
	LeadID       int64             `json:"lead_id"`
	ActivityType AgentActivityType `json:"activity_type"`
	Metadata     map[string]any    `json:"metadata"`
	CreatedAt    string            `json:"created_at"`
}

// EnsureAgentActivitiesTable creates the agent_activities table.
func (d *DB) EnsureAgentActivitiesTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS agent_activities (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			user_id INT NOT NULL,
			org_id INT NOT NULL,
			campaign_id INT DEFAULT NULL,
			lead_id INT DEFAULT NULL,
			activity_type VARCHAR(30) NOT NULL,
			metadata JSON,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_user_created (user_id, created_at),
			INDEX idx_org_created (org_id, created_at),
			INDEX idx_campaign_created (campaign_id, created_at),
			INDEX idx_type_created (activity_type, created_at),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return fmt.Errorf("create agent_activities: %w", err)
	}
	return nil
}

// LogAgentActivity records a single agent action.
func (d *DB) LogAgentActivity(userID, orgID, campaignID, leadID int64, activityType AgentActivityType, metadata map[string]any) error {
	metaJSON, _ := json.Marshal(metadata)
	_, err := d.pool.Exec(`
		INSERT INTO agent_activities (user_id, org_id, campaign_id, lead_id, activity_type, metadata)
		VALUES (?,?,?,?,?,?)`, userID, orgID, nullInt64(campaignID), nullInt64(leadID), string(activityType), metaJSON)
	return err
}

// AgentActivitySummary is the aggregated report row for one agent.
type AgentActivitySummary struct {
	UserID        int64  `json:"user_id"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
	Role          string `json:"role"`
	TotalCalls    int64  `json:"total_calls"`
	Connected     int64  `json:"connected"`
	Completed     int64  `json:"completed"`
	Unanswered    int64  `json:"unanswered"`
	Busy          int64  `json:"busy"`
	Failed        int64  `json:"failed"`
	TotalTalkTime int64  `json:"total_talk_time_s"`
	IdleTime      int64  `json:"idle_time_s"`
	BreakTime     int64  `json:"break_time_s"`
	StatusUpdates int64  `json:"status_updates"`
	NotesAdded    int64  `json:"notes_added"`
	Appointments  int64  `json:"appointments"`
	Conversions   int64  `json:"conversions"`
}

// AgentLeadSummary is the on-screen lead/call summary for one agent/executive.
type AgentLeadSummary struct {
	UserID        int64  `json:"user_id"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
	Role          string `json:"role"`
	AssignedLeads int64  `json:"assigned_leads"`
	NewLeads      int64  `json:"new_leads"`
	CalledLeads   int64  `json:"called_leads"`
	Qualified     int64  `json:"qualified"`
	Appointments  int64  `json:"appointments"`
	TotalCalls    int64  `json:"total_calls"`
	Connected     int64  `json:"connected"`
	Completed     int64  `json:"completed"`
	Unanswered    int64  `json:"unanswered"`
	Busy          int64  `json:"busy"`
	Failed        int64  `json:"failed"`
	Recordings    int64  `json:"recordings"`
	NotesAdded    int64  `json:"notes_added"`
}

// GetAgentLeadSummary returns on-screen performance rows scoped by org,
// optional campaign, user, and date range. Call metrics are attributed to the
// user who made the call, regardless of who the lead is assigned to.
func (d *DB) GetAgentLeadSummary(orgID int64, from, to time.Time, campaignID, userID int64) ([]AgentLeadSummary, error) {
	callCampaignFilter := ""
	outcomeCampaignFilter := ""
	if campaignID > 0 {
		callCampaignFilter = "AND aa.campaign_id=?"
		outcomeCampaignFilter = "AND aa.campaign_id=?"
	}
	userFilter := ""
	if userID > 0 {
		userFilter = "AND u.id=?"
	}

	q := fmt.Sprintf(`
		SELECT
			u.id,
			u.email,
			COALESCE(u.full_name,''),
			COALESCE(u.role,'Agent'),
			0 AS assigned_leads,
			0 AS new_leads,
			0 AS called_leads,
			0 AS qualified,
			COALESCE(outcomes.appointments, 0) AS appointments,
			COALESCE(calls.total_calls, 0) AS total_calls,
			COALESCE(calls.connected, 0) AS connected,
			COALESCE(calls.completed, 0) AS completed,
			COALESCE(calls.unanswered, 0) AS unanswered,
			COALESCE(calls.busy, 0) AS busy,
			COALESCE(calls.failed, 0) AS failed,
			COALESCE(calls.recordings, 0) AS recordings,
			COALESCE(outcomes.notes_added, 0) AS notes_added
		FROM users u
		LEFT JOIN (
			SELECT
				aa.user_id,
				COUNT(*) AS total_calls,
				SUM(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='connected'
					OR (
						JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome')) IN ('unanswered','no_answer')
						AND cl.status IN ('completed','answered','connected')
					) THEN 1 ELSE 0 END) AS connected,
				SUM(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='completed' THEN 1 ELSE 0 END) AS completed,
				SUM(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome')) IN ('unanswered','no_answer')
					AND COALESCE(cl.status,'') NOT IN ('completed','answered','connected','busy','failed','cancelled') THEN 1 ELSE 0 END) AS unanswered,
				SUM(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='busy' OR cl.status='busy' THEN 1 ELSE 0 END) AS busy,
				SUM(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome')) IN ('failed','cancelled') OR cl.status IN ('failed','cancelled') THEN 1 ELSE 0 END) AS failed,
				SUM(CASE WHEN cl.recording_url IS NOT NULL AND cl.recording_url != '' THEN 1 ELSE 0 END) AS recordings
			FROM agent_activities aa
			LEFT JOIN call_logs cl ON cl.org_id=aa.org_id
				AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.call_sid')) COLLATE utf8mb4_0900_ai_ci
			WHERE aa.org_id=?
				AND aa.activity_type='call'
				AND aa.created_at >= ?
				AND aa.created_at <= ?
				%s
			GROUP BY aa.user_id
		) calls ON calls.user_id=u.id
		LEFT JOIN (
			SELECT
				aa.user_id,
				SUM(CASE WHEN aa.activity_type='status_update' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.new_status'))='Appointment Set' THEN 1 ELSE 0 END) AS appointments,
				SUM(CASE WHEN aa.activity_type='note' THEN 1 ELSE 0 END) AS notes_added
			FROM agent_activities aa
			WHERE aa.org_id=?
				AND aa.created_at >= ?
				AND aa.created_at <= ?
				AND aa.activity_type IN ('status_update','note')
				%s
			GROUP BY aa.user_id
		) outcomes ON outcomes.user_id=u.id
		WHERE u.org_id=?
			AND u.role IN ('Agent','Executive','TeamLeader')
			%s
		ORDER BY u.full_name, u.email`, callCampaignFilter, outcomeCampaignFilter, userFilter)

	finalArgs := []any{orgID, from, to}
	if campaignID > 0 {
		finalArgs = append(finalArgs, campaignID)
	}
	finalArgs = append(finalArgs, orgID, from, to)
	if campaignID > 0 {
		finalArgs = append(finalArgs, campaignID)
	}
	finalArgs = append(finalArgs, orgID)
	if userID > 0 {
		finalArgs = append(finalArgs, userID)
	}

	rows, err := d.pool.Query(q, finalArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AgentLeadSummary
	for rows.Next() {
		var s AgentLeadSummary
		err := rows.Scan(&s.UserID, &s.Email, &s.FullName, &s.Role, &s.AssignedLeads, &s.NewLeads,
			&s.CalledLeads, &s.Qualified, &s.Appointments, &s.TotalCalls, &s.Connected, &s.Completed,
			&s.Unanswered, &s.Busy, &s.Failed, &s.Recordings, &s.NotesAdded)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetAgentActivitySummary returns aggregated agent productivity metrics for a date range.
func (d *DB) GetAgentActivitySummary(orgID int64, from, to time.Time, campaignID, userID int64) ([]AgentActivitySummary, error) {
	campaignFilter := ""
	args := []any{orgID, from, to}
	if campaignID > 0 {
		campaignFilter = "AND aa.campaign_id=?"
		args = append(args, campaignID)
	}
	userJoinFilter := ""
	userWhereFilter := ""
	if userID > 0 {
		userWhereFilter = "AND u.id=?"
	}

	finalArgs := append(args, orgID)
	if userID > 0 {
		finalArgs = append(finalArgs, userID)
	}

	rows, err := d.pool.Query(fmt.Sprintf(`
		SELECT
			u.id,
			u.email,
			COALESCE(u.full_name,''),
			COALESCE(u.role,'Agent'),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND (
				JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='connected'
				OR (
					JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome')) IN ('unanswered','no_answer')
					AND cl.status IN ('completed','answered','connected')
				)
			) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call'
				AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome')) IN ('unanswered','no_answer')
				AND COALESCE(cl.status,'') NOT IN ('completed','answered','connected','busy','failed','cancelled') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND (JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='busy' OR cl.status='busy') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND (JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='failed' OR cl.status IN ('failed','cancelled')) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' THEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.duration_s')) ELSE 0 END), 0),
			COALESCE((SELECT p.total_idle_time_s FROM agent_presence p WHERE p.user_id=u.id LIMIT 1), 0) AS idle_time_s,
			COALESCE(SUM(CASE WHEN aa.activity_type='break' THEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.duration_s')) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='status_update' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='note' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='status_update' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.new_status'))='Appointment Set' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='status_update' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.new_status'))='Converted' THEN 1 ELSE 0 END), 0)
		FROM users u
		LEFT JOIN agent_activities aa ON aa.user_id=u.id
			AND aa.org_id=?
			AND aa.created_at >= ?
			AND aa.created_at <= ?
			%s
			%s
		LEFT JOIN call_logs cl ON cl.org_id=aa.org_id
			AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.call_sid')) COLLATE utf8mb4_0900_ai_ci
		WHERE u.org_id=?
			%s
		GROUP BY u.id, u.email, u.full_name, u.role
		ORDER BY u.full_name, u.email`, campaignFilter, userJoinFilter, userWhereFilter), finalArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AgentActivitySummary
	for rows.Next() {
		var s AgentActivitySummary
		err := rows.Scan(&s.UserID, &s.Email, &s.FullName, &s.Role,
			&s.TotalCalls, &s.Connected, &s.Completed, &s.Unanswered, &s.Busy, &s.Failed,
			&s.TotalTalkTime, &s.IdleTime, &s.BreakTime, &s.StatusUpdates, &s.NotesAdded, &s.Appointments, &s.Conversions)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetAgentActivityDetail returns raw activity rows for the report detail sheet.
func (d *DB) GetAgentActivityDetail(orgID int64, from, to time.Time, campaignID, userID int64) ([]AgentActivity, error) {
	campaignFilter := ""
	args := []any{orgID, from, to}
	if campaignID > 0 {
		campaignFilter = "AND campaign_id=?"
		args = append(args, campaignID)
	}
	userFilter := ""
	if userID > 0 {
		userFilter = "AND user_id=?"
		args = append(args, userID)
	}
	rows, err := d.pool.Query(fmt.Sprintf(`
		SELECT id, user_id, org_id, COALESCE(campaign_id,0), COALESCE(lead_id,0),
			activity_type, COALESCE(metadata,'{}'), DATE_FORMAT(created_at,'%%Y-%%m-%%d %%H:%%i:%%s')
		FROM agent_activities
		WHERE org_id=? AND created_at >= ? AND created_at <= ? %s %s
		ORDER BY created_at DESC`, campaignFilter, userFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AgentActivity
	for rows.Next() {
		var a AgentActivity
		var metaJSON string
		err := rows.Scan(&a.ID, &a.UserID, &a.OrgID, &a.CampaignID, &a.LeadID, &a.ActivityType, &metaJSON, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
		list = append(list, a)
	}
	return list, rows.Err()
}

// UserAssignedCampaign is a campaign directly assigned to a dashboard user with
// lead and activity counts.
type UserAssignedCampaign struct {
	CampaignID   int64  `json:"campaign_id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	LeadCount    int64  `json:"lead_count"`
	TotalCalls   int64  `json:"total_calls"`
	Recordings   int64  `json:"recordings"`
	Appointments int64  `json:"appointments"`
	Notes        int64  `json:"notes"`
}

// GetUserAssignedCampaigns returns the campaigns directly assigned to a user
// together with the total leads and the user's call/activity stats for those
// campaigns (all time, not date-bound).
func (d *DB) GetUserAssignedCampaigns(orgID, userID int64) ([]UserAssignedCampaign, error) {
	rows, err := d.pool.Query(`
		SELECT
			c.id,
			c.name,
			COALESCE(c.status, 'active'),
			COALESCE(lc.lead_count, 0),
			COALESCE(cs.total_calls, 0),
			COALESCE(cs.recordings, 0),
			COALESCE(cs.appointments, 0),
			COALESCE(cs.notes, 0)
		FROM campaigns c
		JOIN campaign_user_assignments cua ON cua.campaign_id = c.id AND cua.user_id = ?
		LEFT JOIN (
			SELECT campaign_id, COUNT(*) AS lead_count
			FROM campaign_leads
			GROUP BY campaign_id
		) lc ON lc.campaign_id = c.id
		LEFT JOIN (
			SELECT
				aa.campaign_id,
				SUM(CASE WHEN aa.activity_type = 'call' THEN 1 ELSE 0 END) AS total_calls,
				SUM(CASE WHEN aa.activity_type = 'call'
						AND cl.recording_url IS NOT NULL AND cl.recording_url != '' THEN 1 ELSE 0 END) AS recordings,
				SUM(CASE WHEN aa.activity_type = 'status_update'
						AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.new_status')) = 'Appointment Set' THEN 1 ELSE 0 END) AS appointments,
				SUM(CASE WHEN aa.activity_type = 'note' THEN 1 ELSE 0 END) AS notes
			FROM agent_activities aa
			LEFT JOIN call_logs cl ON cl.org_id = aa.org_id
				AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.call_sid')) COLLATE utf8mb4_0900_ai_ci
			WHERE aa.org_id = ? AND aa.user_id = ?
				AND aa.activity_type IN ('call', 'status_update', 'note')
			GROUP BY aa.campaign_id
		) cs ON cs.campaign_id = c.id
		WHERE c.org_id = ?
		ORDER BY c.name`, userID, orgID, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []UserAssignedCampaign
	for rows.Next() {
		var c UserAssignedCampaign
		if err := rows.Scan(&c.CampaignID, &c.Name, &c.Status, &c.LeadCount,
			&c.TotalCalls, &c.Recordings, &c.Appointments, &c.Notes); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// executiveIDForUser returns the executives.id linked to a dashboard user with
// role Executive via email. It returns 0 when no such executive exists.
func (d *DB) executiveIDForUser(orgID, userID int64) (int64, error) {
	var execID int64
	err := d.pool.QueryRow(`
		SELECT e.id
		FROM executives e
		JOIN users u ON LOWER(u.email) = LOWER(e.email) AND u.org_id = e.org_id
		WHERE e.org_id = ? AND u.id = ? AND u.role = 'Executive'
		LIMIT 1`, orgID, userID).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return execID, err
}

// CountUserAssignedLeads counts leads that are considered assigned to the user:
// either leads in campaigns directly assigned to the user, or leads whose
// campaign_leads.executive_id matches the user's Executive record.
func (d *DB) CountUserAssignedLeads(orgID, userID, campaignID int64) (int64, error) {
	execID, err := d.executiveIDForUser(orgID, userID)
	if err != nil {
		return 0, err
	}
	args := []any{orgID, userID}
	execFilter := ""
	if execID > 0 {
		execFilter = "OR cl.executive_id = ?"
		args = append(args, execID)
	}
	campaignFilter := ""
	if campaignID > 0 {
		campaignFilter = "AND cl.campaign_id = ?"
		args = append(args, campaignID)
	}
	var n int64
	err = d.pool.QueryRow(fmt.Sprintf(`
		SELECT COUNT(DISTINCT l.id)
		FROM leads l
		JOIN campaign_leads cl ON cl.lead_id = l.id
		WHERE l.org_id = ? AND (
			cl.campaign_id IN (SELECT campaign_id FROM campaign_user_assignments WHERE user_id = ?)
			%s
		)
		%s`, execFilter, campaignFilter), args...).Scan(&n)
	return n, err
}

// UserAssignedLead is a lead assigned to a user, including the campaign it was
// pulled from.
type UserAssignedLead struct {
	Lead
	CampaignID   int64  `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
}

// GetUserAssignedLeads returns one page of leads assigned to the user, filtered
// by an optional campaign ID.
func (d *DB) GetUserAssignedLeads(orgID, userID, limit, offset, campaignID int64) ([]UserAssignedLead, error) {
	execID, err := d.executiveIDForUser(orgID, userID)
	if err != nil {
		return nil, err
	}
	args := []any{orgID, userID}
	execFilter := ""
	if execID > 0 {
		execFilter = "OR cl.executive_id = ?"
		args = append(args, execID)
	}
	campaignFilter := ""
	if campaignID > 0 {
		campaignFilter = "AND cl.campaign_id = ?"
		args = append(args, campaignID)
	}
	args = append(args, limit, offset)
	rows, err := d.pool.Query(fmt.Sprintf(`
		SELECT
			l.id, l.org_id, l.first_name, COALESCE(l.last_name, ''), l.phone,
			COALESCE(l.source, ''),
			COALESCE((
				SELECT CASE LOWER(COALESCE(cl.status, ''))
					WHEN 'completed' THEN 'Completed'
					WHEN 'answered' THEN 'Connected'
					WHEN 'connected' THEN 'Connected'
					WHEN 'no-answer' THEN 'No Answer'
					WHEN 'no_answer' THEN 'No Answer'
					WHEN 'busy' THEN 'Busy'
					WHEN 'failed' THEN 'Failed'
					WHEN 'cancelled' THEN 'Cancelled'
					WHEN 'dialing' THEN 'Calling'
					WHEN 'initiated' THEN 'Calling'
					ELSE NULL
				END
				FROM call_logs cl
				WHERE cl.org_id = l.org_id
				  AND cl.lead_id = l.id
				  AND COALESCE(cl.campaign_id, 0) = c.id
				  AND COALESCE(cl.status, '') != ''
				ORDER BY cl.created_at DESC, cl.id DESC
				LIMIT 1
			), CASE WHEN LOWER(COALESCE(l.status, '')) = 'calling' THEN 'None' ELSE COALESCE(l.status, 'new') END),
			COALESCE(l.follow_up_note, ''),
			COALESCE(l.follow_up_at, ''),
			COALESCE(l.interest, ''), COALESCE(l.company, ''), COALESCE(l.external_id, ''),
			COALESCE(l.crm_provider, ''), COALESCE(l.executive_id, 0),
			DATE_FORMAT(l.created_at, '%%Y-%%m-%%d %%H:%%i:%%s'),
			c.id, c.name
		FROM leads l
		JOIN campaign_leads cl ON cl.lead_id = l.id
		JOIN campaigns c ON c.id = cl.campaign_id
		WHERE l.org_id = ? AND (
			cl.campaign_id IN (SELECT campaign_id FROM campaign_user_assignments WHERE user_id = ?)
			%s
		)
		%s
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT ? OFFSET ?`, execFilter, campaignFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []UserAssignedLead
	for rows.Next() {
		var r UserAssignedLead
		var cid int64
		if err := rows.Scan(&r.ID, &r.OrgID, &r.FirstName, &r.LastName, &r.Phone,
			&r.Source, &r.Status, &r.FollowUpNote, &r.FollowUpAt, &r.Interest,
			&r.Company, &r.ExternalID, &r.CRMProvider, &r.ExecutiveID, &r.CreatedAt,
			&cid, &r.CampaignName); err != nil {
			return nil, err
		}
		r.CampaignID = cid
		list = append(list, r)
	}
	return list, rows.Err()
}

// UserRecording is a call recording attributed to a user, enriched with lead
// and campaign names.
type UserRecording struct {
	CallLog
	LeadName     string  `json:"lead_name"`
	CampaignName string  `json:"campaign_name"`
	DurationS    float64 `json:"duration_s"`
	UserCallAt   string  `json:"user_call_at"`
}

// GetUserRecordings returns the user's calls (with recordings) for an optional
// date range, paginated.
func (d *DB) GetUserRecordings(orgID, userID int64, from, to time.Time, limit, offset int64) ([]UserRecording, int64, error) {
	dateFilter := ""
	args := []any{orgID, userID}
	if !from.IsZero() && !to.IsZero() {
		dateFilter = "AND aa.created_at >= ? AND aa.created_at <= ?"
		args = append(args, from, to.Add(24*time.Hour-time.Second))
	} else if !from.IsZero() {
		dateFilter = "AND aa.created_at >= ?"
		args = append(args, from)
	} else if !to.IsZero() {
		dateFilter = "AND aa.created_at <= ?"
		args = append(args, to.Add(24*time.Hour-time.Second))
	}
	countArgs := append([]any(nil), args...)
	listArgs := append([]any(nil), args...)
	listArgs = append(listArgs, limit, offset)

	var total int64
	countQ := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM agent_activities aa
		JOIN call_logs cl ON cl.org_id = aa.org_id
			AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.call_sid')) COLLATE utf8mb4_0900_ai_ci
		WHERE aa.org_id = ? AND aa.user_id = ? AND aa.activity_type = 'call'
			AND cl.recording_url IS NOT NULL AND cl.recording_url != ''
			%s`, dateFilter)
	if err := d.pool.QueryRow(countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := fmt.Sprintf(`
		SELECT
			cl.id, cl.lead_id, COALESCE(cl.campaign_id, 0), cl.org_id,
			COALESCE(cl.call_sid, ''), COALESCE(cl.phone, ''), COALESCE(cl.provider, ''),
			COALESCE(cl.status, ''), COALESCE(cl.recording_url, ''),
			DATE_FORMAT(cl.created_at, '%%Y-%%m-%%d %%H:%%i:%%s'),
			COALESCE(CONCAT(l.first_name, ' ', COALESCE(l.last_name, '')), ''),
			COALESCE(c.name, ''),
			COALESCE(ct.call_duration_s, 0),
			DATE_FORMAT(aa.created_at, '%%Y-%%m-%%d %%H:%%i:%%s')
		FROM agent_activities aa
		JOIN call_logs cl ON cl.org_id = aa.org_id
			AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.call_sid')) COLLATE utf8mb4_0900_ai_ci
		LEFT JOIN leads l ON l.id = cl.lead_id
		LEFT JOIN campaigns c ON c.id = cl.campaign_id
		LEFT JOIN call_transcripts ct ON ct.call_sid COLLATE utf8mb4_0900_ai_ci = cl.call_sid
		WHERE aa.org_id = ? AND aa.user_id = ? AND aa.activity_type = 'call'
			AND cl.recording_url IS NOT NULL AND cl.recording_url != ''
			%s
		ORDER BY aa.created_at DESC
		LIMIT ? OFFSET ?`, dateFilter)
	rows, err := d.pool.Query(q, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []UserRecording
	for rows.Next() {
		var r UserRecording
		if err := rows.Scan(&r.ID, &r.LeadID, &r.CampaignID, &r.OrgID,
			&r.CallSid, &r.Phone, &r.Provider, &r.Status, &r.RecordingURL, &r.CreatedAt,
			&r.LeadName, &r.CampaignName, &r.DurationS, &r.UserCallAt); err != nil {
			return nil, 0, err
		}
		list = append(list, r)
	}
	return list, total, rows.Err()
}

// UserActivityStats aggregates the user's activity counts.
type UserActivityStats struct {
	AssignedLeads     int64 `json:"assigned_leads"`
	AssignedCampaigns int64 `json:"assigned_campaigns"`
	TotalCalls        int64 `json:"total_calls"`
	Recordings        int64 `json:"recordings"`
	Appointments      int64 `json:"appointments"`
	Notes             int64 `json:"notes"`
}

// GetUserActivityStats returns call/outcome/note counts for the user, optionally
// filtered by a date range. AssignedLeads and AssignedCampaigns are populated
// by the caller from the dedicated helpers.
func (d *DB) GetUserActivityStats(orgID, userID int64, from, to time.Time) (UserActivityStats, error) {
	var s UserActivityStats
	dateFilter := ""
	args := []any{orgID, userID}
	if !from.IsZero() && !to.IsZero() {
		dateFilter = "AND aa.created_at >= ? AND aa.created_at <= ?"
		args = append(args, from, to.Add(24*time.Hour-time.Second))
	} else if !from.IsZero() {
		dateFilter = "AND aa.created_at >= ?"
		args = append(args, from)
	} else if !to.IsZero() {
		dateFilter = "AND aa.created_at <= ?"
		args = append(args, to.Add(24*time.Hour-time.Second))
	}
	err := d.pool.QueryRow(fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN aa.activity_type = 'call' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type = 'call'
					AND cl.recording_url IS NOT NULL AND cl.recording_url != '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type = 'status_update'
					AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.new_status')) = 'Appointment Set' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type = 'note' THEN 1 ELSE 0 END), 0)
		FROM agent_activities aa
		LEFT JOIN call_logs cl ON cl.org_id = aa.org_id
			AND cl.call_sid = JSON_UNQUOTE(JSON_EXTRACT(aa.metadata, '$.call_sid')) COLLATE utf8mb4_0900_ai_ci
		WHERE aa.org_id = ? AND aa.user_id = ?
			AND aa.activity_type IN ('call', 'status_update', 'note')
			%s`, dateFilter), args...).Scan(&s.TotalCalls, &s.Recordings, &s.Appointments, &s.Notes)
	return s, err
}

// UserDetailResponse is the payload for GET /api/analytics/user-detail/{id}.
type UserDetailResponse struct {
	Profile        User                   `json:"profile"`
	Stats          UserActivityStats      `json:"stats"`
	Campaigns      []UserAssignedCampaign `json:"campaigns"`
	Leads          []UserAssignedLead     `json:"leads"`
	Recordings     []UserRecording        `json:"recordings"`
	LeadPagination struct {
		Page  int64 `json:"page"`
		Limit int64 `json:"limit"`
		Total int64 `json:"total"`
	} `json:"lead_pagination"`
	RecordingPagination struct {
		Page  int64 `json:"page"`
		Limit int64 `json:"limit"`
		Total int64 `json:"total"`
	} `json:"recording_pagination"`
}
