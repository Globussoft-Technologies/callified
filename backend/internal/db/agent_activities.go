package db

import (
	"encoding/json"
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

// GetAgentActivitySummary returns aggregated agent productivity metrics for a date range.
func (d *DB) GetAgentActivitySummary(orgID int64, from, to time.Time, campaignID, userID int64) ([]AgentActivitySummary, error) {
	campaignFilter := ""
	args := []any{orgID, from, to}
	if campaignID > 0 {
		campaignFilter = "AND aa.campaign_id=?"
		args = append(args, campaignID)
	}
	userFilter := ""
	if userID > 0 {
		userFilter = "AND aa.user_id=?"
		args = append(args, userID)
	}

	rows, err := d.pool.Query(fmt.Sprintf(`
		SELECT
			u.id,
			u.email,
			COALESCE(u.full_name,''),
			COALESCE(u.role,'Agent'),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='connected' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='unanswered' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='busy' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' AND JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.outcome'))='failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN aa.activity_type='call' THEN JSON_UNQUOTE(JSON_EXTRACT(aa.metadata,'$.duration_s')) ELSE 0 END), 0),
			(SELECT COALESCE(p.total_idle_time_s,0) FROM agent_presence p WHERE p.user_id=u.id) AS idle_time_s,
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
		WHERE u.org_id=?
		GROUP BY u.id, u.email, u.full_name, u.role
		ORDER BY u.full_name, u.email`, campaignFilter, userFilter), append(args, orgID)...)
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
