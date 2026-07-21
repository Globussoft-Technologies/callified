package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Role constants for the dashboard RBAC model.
const (
	RoleAdmin      = "Admin"
	RoleTeamLeader = "TeamLeader"
	RoleAgent      = "Agent"
)

// EnsureRBACTables creates/extends tables needed for role-based access and
// campaign-user assignments. It follows the same defensive pattern as the
// other Ensure*Table helpers: ALTER statements that fail with "Duplicate"
// are ignored so the migration is idempotent.
func (d *DB) EnsureRBACTables() error {
	// Extend users with manager link and active flag.
	userCols := []struct{ name, def string }{
		{"manager_id", "BIGINT DEFAULT NULL"},
		{"is_active", "TINYINT(1) NOT NULL DEFAULT 1"},
	}
	for _, col := range userCols {
		_, err := d.pool.Exec(fmt.Sprintf("ALTER TABLE users ADD COLUMN %s %s", col.name, col.def))
		if err != nil && !strings.Contains(err.Error(), "Duplicate column name") {
			return fmt.Errorf("add users.%s: %w", col.name, err)
		}
	}
	_, err := d.pool.Exec(`CREATE INDEX idx_manager_id ON users(manager_id)`)
	if err != nil && !strings.Contains(err.Error(), "Duplicate") {
		return fmt.Errorf("add idx_manager_id: %w", err)
	}

	// Campaign-to-user assignments. campaign_id/user_id match the INT type of
	// the referenced campaigns/users tables on existing schemas.
	_, err = d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS campaign_user_assignments (
			campaign_id INT NOT NULL,
			user_id INT NOT NULL,
			assigned_by INT DEFAULT NULL,
			assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (campaign_id, user_id),
			INDEX idx_user_id (user_id),
			INDEX idx_campaign_id (campaign_id),
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return fmt.Errorf("create campaign_user_assignments: %w", err)
	}

	// Persistent in-app notifications.
	_, err = d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			user_id INT NOT NULL,
			type VARCHAR(50) NOT NULL,
			title VARCHAR(255) NOT NULL,
			body TEXT,
			payload JSON,
			is_read TINYINT(1) NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_user_id_read (user_id, is_read),
			INDEX idx_created_at (created_at DESC)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return fmt.Errorf("create notifications: %w", err)
	}

	// Backfill users that don't have a recognized admin role so existing
	// accounts default to Agent (the lowest-privilege dashboard role).
	_, err = d.pool.Exec(`UPDATE users SET role='Agent' WHERE role IS NULL OR role NOT IN ('Admin','SuperAdmin','TeamLeader','Agent')`)
	if err != nil {
		return fmt.Errorf("backfill default roles: %w", err)
	}

	return nil
}

// CreateManagedUser inserts a user created directly by an Admin or Team Leader
// (as opposed to the email-invite flow). managerID may be nil.
func (d *DB) CreateManagedUser(email, passwordHash, fullName, role string, orgID int64, managerID *int64) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO users (email, password_hash, full_name, role, org_id, manager_id) VALUES (?,?,?,?,?,?)`,
		email, passwordHash, fullName, role, nullInt64(orgID), nullInt64Ptr(managerID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUsersByOrg returns all dashboard users for an org with their RBAC fields.
func (d *DB) GetUsersByOrg(orgID int64) ([]User, error) {
	rows, err := d.pool.Query(
		`SELECT id, COALESCE(org_id,0), email, '', COALESCE(full_name,''), COALESCE(role,'Agent'),
		        manager_id, COALESCE(is_active,1),
		        COALESCE(DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%sZ'), '')
		 FROM users WHERE org_id=? ORDER BY full_name, email`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

// GetAgentsByManager returns the Agents whose manager_id equals managerID.
func (d *DB) GetAgentsByManager(managerID int64) ([]User, error) {
	rows, err := d.pool.Query(
		`SELECT id, COALESCE(org_id,0), email, '', COALESCE(full_name,''), COALESCE(role,'Agent'),
		        manager_id, COALESCE(is_active,1),
		        COALESCE(DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%sZ'), '')
		 FROM users WHERE manager_id=? ORDER BY full_name, email`, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

// GetUserByIDInOrgWithRole returns a user scoped to an org, including manager_id
// and is_active. Returns nil when not found.
func (d *DB) GetUserByIDInOrgWithRole(userID, orgID int64) (*User, error) {
	row := d.pool.QueryRow(
		`SELECT id, COALESCE(org_id,0), email, '', COALESCE(full_name,''), COALESCE(role,'Agent'),
		        manager_id, COALESCE(is_active,1),
		        COALESCE(DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%sZ'), '')
		 FROM users WHERE id=? AND org_id=?`, userID, orgID)
	u := &User{}
	var managerID sql.NullInt64
	err := row.Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &managerID, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if managerID.Valid {
		u.ManagerID = &managerID.Int64
	}
	return u, nil
}

// UpdateUser updates editable user fields. Admin-only scope.
func (d *DB) UpdateUser(userID, orgID int64, fullName, role string, managerID *int64, isActive bool) error {
	_, err := d.pool.Exec(
		`UPDATE users SET full_name=?, role=?, manager_id=?, is_active=? WHERE id=? AND org_id=?`,
		fullName, role, nullInt64Ptr(managerID), isActive, userID, orgID)
	return err
}

// UpdateUserActive toggles the is_active flag for a user. Used by Admin or a
// Team Leader for their own reports.
func (d *DB) UpdateUserActive(userID int64, isActive bool) error {
	_, err := d.pool.Exec(`UPDATE users SET is_active=? WHERE id=?`, isActive, userID)
	return err
}

// SetUserManager sets (or clears) an Agent's Team Leader.
func (d *DB) SetUserManager(userID int64, managerID *int64) error {
	_, err := d.pool.Exec(`UPDATE users SET manager_id=? WHERE id=?`, nullInt64Ptr(managerID), userID)
	return err
}

// GetManagedUserIDs returns the user IDs of active Agents under a manager.
func (d *DB) GetManagedUserIDs(managerID int64) ([]int64, error) {
	rows, err := d.pool.Query(`SELECT id FROM users WHERE manager_id=? AND role='Agent' AND is_active=1`, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// IsUserManagedBy reports whether targetUserID is an Agent whose manager is
// managerID. Used by Team Leader endpoints to scope edits to their own reports.
func (d *DB) IsUserManagedBy(targetUserID, managerID int64) (bool, error) {
	var n int
	err := d.pool.QueryRow(
		`SELECT COUNT(*) FROM users WHERE id=? AND manager_id=?`, targetUserID, managerID).Scan(&n)
	return n > 0, err
}

// CountUsersByOrgAndIDs returns how many of the supplied user IDs belong to
// the given org. Used to reject campaign assignments that reference foreign users.
func (d *DB) CountUsersByOrgAndIDs(orgID int64, userIDs []int64) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	placeholders := strings.Repeat("?,", len(userIDs)-1) + "?"
	args := make([]any, 0, len(userIDs)+1)
	args = append(args, orgID)
	for _, id := range userIDs {
		args = append(args, id)
	}
	var count int64
	err := d.pool.QueryRow(
		"SELECT COUNT(*) FROM users WHERE org_id=? AND id IN ("+placeholders+")", args...).Scan(&count)
	return count, err
}

// AssignCampaignToUsers replaces all user assignments for a campaign.
func (d *DB) AssignCampaignToUsers(campaignID int64, userIDs []int64, assignedBy int64) error {
	tx, err := d.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM campaign_user_assignments WHERE campaign_id=?`, campaignID); err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return tx.Commit()
	}

	placeholders := strings.Repeat("(?,?,?),", len(userIDs)-1) + "(?,?,?)"
	q := `INSERT INTO campaign_user_assignments (campaign_id, user_id, assigned_by) VALUES ` + placeholders
	args := make([]any, 0, len(userIDs)*3)
	for _, uid := range userIDs {
		args = append(args, campaignID, uid, assignedBy)
	}
	if _, err := tx.Exec(q, args...); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAssignedUserIDsForCampaign returns the user IDs assigned to a campaign.
func (d *DB) GetAssignedUserIDsForCampaign(campaignID int64) ([]int64, error) {
	rows, err := d.pool.Query(
		`SELECT user_id FROM campaign_user_assignments WHERE campaign_id=? ORDER BY user_id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetCampaignsByUserIDs returns campaign IDs assigned to any of the given users.
func (d *DB) GetCampaignsByUserIDs(userIDs []int64) ([]int64, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(userIDs)-1) + "?"
	rows, err := d.pool.Query(
		`SELECT DISTINCT campaign_id FROM campaign_user_assignments WHERE user_id IN (`+placeholders+`) ORDER BY campaign_id`,
		func() []any {
			out := make([]any, len(userIDs))
			for i, id := range userIDs {
				out[i] = id
			}
			return out
		}()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetCampaignsForUser returns the campaign IDs directly assigned to a user.
func (d *DB) GetCampaignsForUser(userID int64) ([]int64, error) {
	return d.GetCampaignsByUserIDs([]int64{userID})
}

// GetCampaignsForManager returns the campaign IDs assigned to a Team Leader
// or to any Agent they manage.
func (d *DB) GetCampaignsForManager(managerID int64) ([]int64, error) {
	managed, err := d.GetManagedUserIDs(managerID)
	if err != nil {
		return nil, err
	}
	all := append([]int64{managerID}, managed...)
	return d.GetCampaignsByUserIDs(all)
}

// IsCampaignAssignedToUser reports whether a campaign is directly assigned.
func (d *DB) IsCampaignAssignedToUser(campaignID, userID int64) (bool, error) {
	var n int
	err := d.pool.QueryRow(
		`SELECT COUNT(*) FROM campaign_user_assignments WHERE campaign_id=? AND user_id=?`,
		campaignID, userID).Scan(&n)
	return n > 0, err
}

// IsCampaignAssignedToManager reports whether a campaign is assigned to the
// manager or any of their managed Agents.
func (d *DB) IsCampaignAssignedToManager(campaignID, managerID int64) (bool, error) {
	managed, err := d.GetManagedUserIDs(managerID)
	if err != nil {
		return false, err
	}
	all := append([]int64{managerID}, managed...)
	placeholders := strings.Repeat("?,", len(all)-1) + "?"
	args := make([]any, 0, len(all)+1)
	args = append(args, campaignID)
	for _, id := range all {
		args = append(args, id)
	}
	var n int
	err = d.pool.QueryRow(
		`SELECT COUNT(*) FROM campaign_user_assignments WHERE campaign_id=? AND user_id IN (`+placeholders+`)`,
		args...).Scan(&n)
	return n > 0, err
}

// Notification mirrors the notifications table.
type Notification struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Payload   string `json:"payload,omitempty"`
	IsRead    bool   `json:"is_read"`
	CreatedAt string `json:"created_at"`
}

// CreateNotification inserts a notification row.
func (d *DB) CreateNotification(userID int64, nType, title, body, payload string) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO notifications (user_id, type, title, body, payload) VALUES (?,?,?,?,?)`,
		userID, nType, title, body, nullString(payload))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetNotificationsForUser returns a user's notifications, newest first.
func (d *DB) GetNotificationsForUser(userID int64, limit int64) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.pool.Query(
		`SELECT id, user_id, type, title, body, COALESCE(payload,''), is_read,
		        DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%sZ')
		 FROM notifications
		 WHERE user_id=?
		 ORDER BY created_at DESC
		 LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Payload, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// GetUnreadNotificationCount returns the number of unread notifications.
func (d *DB) GetUnreadNotificationCount(userID int64) (int64, error) {
	var n int64
	err := d.pool.QueryRow(
		`SELECT COUNT(*) FROM notifications WHERE user_id=? AND is_read=0`, userID).Scan(&n)
	return n, err
}

// MarkNotificationRead marks a single notification as read, scoped to the owner.
func (d *DB) MarkNotificationRead(notificationID, userID int64) error {
	_, err := d.pool.Exec(
		`UPDATE notifications SET is_read=1 WHERE id=? AND user_id=?`, notificationID, userID)
	return err
}

// MarkAllNotificationsRead marks all notifications for a user as read.
func (d *DB) MarkAllNotificationsRead(userID int64) error {
	_, err := d.pool.Exec(`UPDATE notifications SET is_read=1 WHERE user_id=?`, userID)
	return err
}

// scanUsers scans rows into []User, handling manager_id as a nullable pointer.
func scanUsers(rows *sql.Rows) ([]User, error) {
	var list []User
	for rows.Next() {
		var u User
		var managerID sql.NullInt64
		err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &managerID, &u.IsActive, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		if managerID.Valid {
			u.ManagerID = &managerID.Int64
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// nullInt64Ptr converts a *int64 to sql.NullInt64.
func nullInt64Ptr(i *int64) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *i, Valid: true}
}
