package db

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// GetUserPermissions returns the saved custom permission keys for a user.
// The boolean is false when the user has no custom override yet.
func (d *DB) GetUserPermissions(userID, orgID int64) ([]string, bool, error) {
	var raw string
	err := d.pool.QueryRow(
		`SELECT permissions FROM user_permissions WHERE user_id=? AND org_id=?`,
		userID, orgID,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		return nil, false, err
	}
	return perms, true, nil
}

// SetUserPermissions stores the exact custom permission keys for a user.
func (d *DB) SetUserPermissions(userID, orgID int64, permissions []string) error {
	raw, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	_, err = d.pool.Exec(
		`INSERT INTO user_permissions (user_id, org_id, permissions)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE org_id=VALUES(org_id), permissions=VALUES(permissions), updated_at=CURRENT_TIMESTAMP`,
		userID, orgID, string(raw),
	)
	return err
}
