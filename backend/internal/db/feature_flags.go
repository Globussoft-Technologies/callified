package db

import (
	"database/sql"
	"errors"
	"time"
)

// UserFeatureFlag mirrors a row in user_feature_flags.
type UserFeatureFlag struct {
	Email          string    `json:"email"`
	HideAiFeatures bool      `json:"hide_ai_features"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EnsureUserFeatureFlagsTable creates the user_feature_flags table if it doesn't exist.
func (d *DB) EnsureUserFeatureFlagsTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS user_feature_flags (
			email VARCHAR(255) PRIMARY KEY,
			hide_ai_features BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_hide_ai_features (hide_ai_features)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	return err
}

// GetUserFeatureFlag fetches feature flags for an email. Returns nil, nil when not found.
func (d *DB) GetUserFeatureFlag(email string) (*UserFeatureFlag, error) {
	row := d.pool.QueryRow(
		`SELECT email, hide_ai_features, created_at, updated_at
		 FROM user_feature_flags WHERE email = ?`, email)
	f := &UserFeatureFlag{}
	var createdAt sql.NullTime
	var updatedAt sql.NullTime
	err := row.Scan(&f.Email, &f.HideAiFeatures, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		f.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		f.UpdatedAt = updatedAt.Time
	}
	return f, nil
}

// ShouldHideAiFeatures returns true if the given email has AI features hidden.
func (d *DB) ShouldHideAiFeatures(email string) bool {
	flag, err := d.GetUserFeatureFlag(email)
	if err != nil || flag == nil {
		return false
	}
	return flag.HideAiFeatures
}

// SetUserFeatureFlag creates or updates feature flags for an email.
func (d *DB) SetUserFeatureFlag(email string, hideAiFeatures bool) error {
	_, err := d.pool.Exec(
		`INSERT INTO user_feature_flags (email, hide_ai_features) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE hide_ai_features = VALUES(hide_ai_features)`,
		email, hideAiFeatures,
	)
	return err
}

// DeleteUserFeatureFlag removes feature flags for an email.
func (d *DB) DeleteUserFeatureFlag(email string) error {
	_, err := d.pool.Exec(`DELETE FROM user_feature_flags WHERE email = ?`, email)
	return err
}
