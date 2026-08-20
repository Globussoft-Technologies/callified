package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PresenceStatus values for the agent dashboard.
const (
	PresenceOffline = "offline"
	PresenceIdle    = "idle"
	PresenceBreak   = "break"
	PresenceOnCall  = "on_call"
)

// AgentPresenceRow is one agent's live status record.
type AgentPresenceRow struct {
	UserID        int64  `json:"user_id"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
	Role          string `json:"role"`
	ManagerID     *int64 `json:"manager_id,omitempty"`
	Status        string `json:"status"`
	LastSeenAt    string `json:"last_seen_at"`
	OnCallSince   string `json:"on_call_since"`
	BreakSince    string `json:"break_since"`
	IdleSince     string `json:"idle_since"`
	TotalTalkTime int64  `json:"total_talk_time_s"`
	TotalIdleTime int64  `json:"total_idle_time_s"`
}

// EnsureAgentPresenceTable creates the agent_presence table and supporting index.
func (d *DB) EnsureAgentPresenceTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS agent_presence (
			user_id INT PRIMARY KEY,
			status VARCHAR(20) NOT NULL DEFAULT 'offline',
			last_seen_at DATETIME DEFAULT NULL,
			on_call_since DATETIME DEFAULT NULL,
			break_since DATETIME DEFAULT NULL,
			idle_since DATETIME DEFAULT NULL,
			total_talk_time_s BIGINT NOT NULL DEFAULT 0,
			total_idle_time_s BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`)
	if err != nil {
		return fmt.Errorf("create agent_presence: %w", err)
	}
	_, _ = d.pool.Exec(`CREATE INDEX idx_agent_presence_status ON agent_presence(status)`)
	// Add columns that may be missing on legacy schemas.
	_, _ = d.pool.Exec(`ALTER TABLE agent_presence ADD COLUMN total_talk_time_s BIGINT NOT NULL DEFAULT 0`)
	_, _ = d.pool.Exec(`ALTER TABLE agent_presence ADD COLUMN total_idle_time_s BIGINT NOT NULL DEFAULT 0`)
	_, _ = d.pool.Exec(`ALTER TABLE agent_presence ADD COLUMN idle_since DATETIME DEFAULT NULL`)
	return nil
}

// UpsertAgentPresence writes or updates an agent's status.
// idleDeltaSeconds is added to the cumulative idle-time total (used when the
// agent transitions out of idle).
func (d *DB) UpsertAgentPresence(userID int64, status string, onCallSince, breakSince, idleSince *time.Time, idleDeltaSeconds int64) error {
	var onCallNull, breakNull, idleNull sql.NullTime
	if onCallSince != nil {
		onCallNull = sql.NullTime{Time: *onCallSince, Valid: true}
	}
	if breakSince != nil {
		breakNull = sql.NullTime{Time: *breakSince, Valid: true}
	}
	if idleSince != nil {
		idleNull = sql.NullTime{Time: *idleSince, Valid: true}
	}
	_, err := d.pool.Exec(`
		INSERT INTO agent_presence (user_id, status, last_seen_at, on_call_since, break_since, idle_since, total_idle_time_s)
		VALUES (?, ?, UTC_TIMESTAMP(), ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			last_seen_at = VALUES(last_seen_at),
			on_call_since = VALUES(on_call_since),
			break_since = VALUES(break_since),
			idle_since = VALUES(idle_since),
			total_idle_time_s = total_idle_time_s + VALUES(total_idle_time_s)`,
		userID, status, onCallNull, breakNull, idleNull, idleDeltaSeconds)
	return err
}

// TouchAgentPresence updates last_seen_at and bumps an offline agent to idle.
func (d *DB) TouchAgentPresence(userID int64, status string) error {
	if status == "" {
		status = PresenceIdle
	}
	_, err := d.pool.Exec(`
		INSERT INTO agent_presence (user_id, status, last_seen_at)
		VALUES (?, ?, UTC_TIMESTAMP())
		ON DUPLICATE KEY UPDATE
			last_seen_at = VALUES(last_seen_at),
			status = IF(status = 'offline' OR status = '', VALUES(status), status)`,
		userID, status)
	return err
}

// AddAgentTalkTime increments an agent's cumulative talk time by the given seconds.
func (d *DB) AddAgentTalkTime(userID int64, seconds int64) error {
	_, err := d.pool.Exec(`
		INSERT INTO agent_presence (user_id, total_talk_time_s, last_seen_at)
		VALUES (?, ?, UTC_TIMESTAMP())
		ON DUPLICATE KEY UPDATE
			total_talk_time_s = total_talk_time_s + VALUES(total_talk_time_s),
			last_seen_at = VALUES(last_seen_at)`, userID, seconds)
	return err
}

// MarkAgentsOffline sets any agent whose last_seen_at is older than cutoff to offline.
// Any idle time accumulated up to the cutoff is added to total_idle_time_s.
func (d *DB) MarkAgentsOffline(cutoff time.Time) error {
	_, err := d.pool.Exec(`
		UPDATE agent_presence
		SET status='offline',
		    on_call_since=NULL,
		    break_since=NULL,
		    idle_since=NULL,
		    total_idle_time_s = total_idle_time_s + COALESCE(TIMESTAMPDIFF(SECOND, idle_since, UTC_TIMESTAMP()), 0)
		WHERE (last_seen_at < ? OR last_seen_at IS NULL) AND status != 'offline'`, cutoff)
	return err
}

// GetAgentPresence returns a single user's presence row, or nil if not present.
func (d *DB) GetAgentPresence(userID int64) (*AgentPresenceRow, error) {
	row := d.pool.QueryRow(`
			SELECT u.id, u.email, COALESCE(u.full_name,''), COALESCE(u.role,'Agent'), u.manager_id,
			COALESCE(p.status,'offline'),
			COALESCE(DATE_FORMAT(p.last_seen_at,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.on_call_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.break_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.idle_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(p.total_talk_time_s,0),
			COALESCE(p.total_idle_time_s,0)
		FROM users u
		LEFT JOIN agent_presence p ON p.user_id=u.id
		WHERE u.id=?`, userID)
	r := &AgentPresenceRow{}
	var managerID sql.NullInt64
	err := row.Scan(&r.UserID, &r.Email, &r.FullName, &r.Role, &managerID,
		&r.Status, &r.LastSeenAt, &r.OnCallSince, &r.BreakSince, &r.IdleSince,
		&r.TotalTalkTime, &r.TotalIdleTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if managerID.Valid {
		r.ManagerID = &managerID.Int64
	}
	return r, nil
}

// GetAgentPresenceByOrg returns live presence rows for every user in the org.
func (d *DB) GetAgentPresenceByOrg(orgID int64) ([]AgentPresenceRow, error) {
	rows, err := d.pool.Query(`
			SELECT u.id, u.email, COALESCE(u.full_name,''), COALESCE(u.role,'Agent'), u.manager_id,
			COALESCE(p.status,'offline'),
			COALESCE(DATE_FORMAT(p.last_seen_at,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.on_call_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.break_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(DATE_FORMAT(p.idle_since,'%Y-%m-%dT%H:%i:%sZ'),''),
			COALESCE(p.total_talk_time_s,0),
			COALESCE(p.total_idle_time_s,0)
		FROM users u
		LEFT JOIN agent_presence p ON p.user_id=u.id
		WHERE u.org_id=?
		ORDER BY u.full_name, u.email`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AgentPresenceRow
	for rows.Next() {
		var r AgentPresenceRow
		var managerID sql.NullInt64
		err := rows.Scan(&r.UserID, &r.Email, &r.FullName, &r.Role, &managerID,
			&r.Status, &r.LastSeenAt, &r.OnCallSince, &r.BreakSince, &r.IdleSince,
			&r.TotalTalkTime, &r.TotalIdleTime)
		if err != nil {
			return nil, err
		}
		if managerID.Valid {
			r.ManagerID = &managerID.Int64
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
