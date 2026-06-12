package db

import (
	"database/sql"
	"errors"
	"time"
)

// AdminSubscription mirrors the admin_subscriptions table row.
type AdminSubscription struct {
	ID         int64     `json:"id"`
	AdminEmail string    `json:"admin_email"`
	ExpiresAt  time.Time `json:"expires_at"`
	Plan       string    `json:"plan"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EnsureAdminSubscriptionsTable creates the admin_subscriptions table if it doesn't exist.
func (d *DB) EnsureAdminSubscriptionsTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS admin_subscriptions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			admin_email VARCHAR(255) NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			plan VARCHAR(50) DEFAULT 'standard',
			is_active BOOLEAN DEFAULT TRUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_admin_email (admin_email),
			INDEX idx_expires_at (expires_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	return err
}

// GetAdminSubscriptionByEmail fetches a subscription by admin email. Returns nil, nil when not found.
func (d *DB) GetAdminSubscriptionByEmail(email string) (*AdminSubscription, error) {
	row := d.pool.QueryRow(
		`SELECT id, admin_email, expires_at, COALESCE(plan,'standard'), is_active, created_at, updated_at
		 FROM admin_subscriptions WHERE admin_email = ?`, email)
	s := &AdminSubscription{}
	var expiresAt sql.NullTime
	var createdAt sql.NullTime
	var updatedAt sql.NullTime
	err := row.Scan(&s.ID, &s.AdminEmail, &expiresAt, &s.Plan, &s.IsActive, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		s.ExpiresAt = expiresAt.Time
	}
	if createdAt.Valid {
		s.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		s.UpdatedAt = updatedAt.Time
	}
	return s, nil
}

// CreateAdminSubscription inserts a new subscription.
func (d *DB) CreateAdminSubscription(adminEmail string, expiresAt time.Time, plan string) (int64, error) {
	if plan == "" {
		plan = "standard"
	}
	res, err := d.pool.Exec(
		`INSERT INTO admin_subscriptions (admin_email, expires_at, plan) VALUES (?, ?, ?)`,
		adminEmail, expiresAt, plan,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAdminSubscription updates an existing subscription by admin email.
func (d *DB) UpdateAdminSubscription(adminEmail string, expiresAt time.Time, plan string, isActive bool) error {
	if plan == "" {
		plan = "standard"
	}
	_, err := d.pool.Exec(
		`UPDATE admin_subscriptions SET expires_at = ?, plan = ?, is_active = ? WHERE admin_email = ?`,
		expiresAt, plan, isActive, adminEmail,
	)
	return err
}

// CreateOrUpdateAdminSubscription creates a subscription if it doesn't exist, otherwise updates it.
func (d *DB) CreateOrUpdateAdminSubscription(adminEmail string, expiresAt time.Time, plan string, isActive bool) error {
	existing, err := d.GetAdminSubscriptionByEmail(adminEmail)
	if err != nil {
		return err
	}
	if existing == nil {
		_, err = d.CreateAdminSubscription(adminEmail, expiresAt, plan)
		return err
	}
	return d.UpdateAdminSubscription(adminEmail, expiresAt, plan, isActive)
}

// AdminSubscriptionStatus returns the subscription status for a user.
type AdminSubscriptionStatus struct {
	Found     bool      `json:"found"`
	Active    bool      `json:"active"`
	Expired   bool      `json:"expired"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Plan      string    `json:"plan,omitempty"`
}

// ValidateAdminSubscription checks whether the given admin email has an active, non-expired subscription.
func (d *DB) ValidateAdminSubscription(email string) (*AdminSubscriptionStatus, error) {
	sub, err := d.GetAdminSubscriptionByEmail(email)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return &AdminSubscriptionStatus{Found: false}, nil
	}
	now := time.Now().UTC()
	expired := sub.ExpiresAt.Before(now) || sub.ExpiresAt.Equal(now)
	return &AdminSubscriptionStatus{
		Found:     true,
		Active:    sub.IsActive && !expired,
		Expired:   expired,
		ExpiresAt: sub.ExpiresAt,
		Plan:      sub.Plan,
	}, nil
}
