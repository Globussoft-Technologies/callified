// Package db provides a MySQL data layer for the Callified REST API.
// All functions mirror the query semantics of database.py.
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DB wraps *sql.DB and provides all query methods.
type DB struct {
	pool *sql.DB
}

// PoolConfig tunes the MySQL connection pool. Zero values are replaced with
// sensible defaults so callers can override only the knobs they care about.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (pc *PoolConfig) applyDefaults() {
	if pc.MaxOpenConns <= 0 {
		pc.MaxOpenConns = 25
	}
	if pc.MaxIdleConns <= 0 {
		pc.MaxIdleConns = 10
	}
	if pc.ConnMaxLifetime <= 0 {
		pc.ConnMaxLifetime = 5 * time.Minute
	}
}

// New opens a MySQL connection pool from the given DSN and verifies connectivity.
// DSN format: "user:password@tcp(host:3306)/dbname?parseTime=true"
func New(dsn string, poolConfig PoolConfig) (*DB, error) {
	poolConfig.applyDefaults()
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.New: open: %w", err)
	}
	pool.SetMaxOpenConns(poolConfig.MaxOpenConns)
	pool.SetMaxIdleConns(poolConfig.MaxIdleConns)
	pool.SetConnMaxLifetime(poolConfig.ConnMaxLifetime)
	if err := pool.Ping(); err != nil {
		return nil, fmt.Errorf("db.New: ping: %w", err)
	}
	d := &DB{pool: pool}
	if err := d.EnsureOrganizationsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure organizations table: %w", err)
	}
	if err := d.EnsureAdminSubscriptionsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure admin subscriptions table: %w", err)
	}
	if err := d.EnsureUserFeatureFlagsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure user feature flags table: %w", err)
	}
	if err := d.EnsureOrgExotelAccountsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure org exotel accounts table: %w", err)
	}
	if err := d.EnsureUserAllowedExotelAccountsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure user allowed exotel accounts table: %w", err)
	}
	if err := d.EnsureProductsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure products table: %w", err)
	}
	if err := d.EnsureCampaignsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure campaigns table: %w", err)
	}
	if err := d.EnsureExecutivesTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure executives table: %w", err)
	}
	if err := d.EnsureScheduledCallsTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure scheduled calls table: %w", err)
	}
	if err := d.EnsureRBACTables(); err != nil {
		return nil, fmt.Errorf("db.New: ensure RBAC tables: %w", err)
	}
	if err := d.EnsureAgentPresenceTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure agent presence table: %w", err)
	}
	if err := d.EnsureAgentActivitiesTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure agent activities table: %w", err)
	}
	if err := d.EnsureCallTranscriptColumns(); err != nil {
		return nil, fmt.Errorf("db.New: ensure call transcript columns: %w", err)
	}
	if err := d.EnsureAPIKeysTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure API keys table: %w", err)
	}
	if err := d.EnsurePromptTemplatesTable(); err != nil {
		return nil, fmt.Errorf("db.New: ensure prompt templates table: %w", err)
	}
	if err := d.EnsurePhase5Indexes(); err != nil {
		return nil, fmt.Errorf("db.New: ensure phase 5 indexes: %w", err)
	}
	return d, nil
}

// Close closes the underlying connection pool.
func (d *DB) Close() error { return d.pool.Close() }

// Ping verifies the database connection is still alive.
func (d *DB) Ping() error { return d.pool.Ping() }

// nullString converts an empty string to sql.NullString.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// nullInt64 converts 0 to sql.NullInt64.
func nullInt64(i int64) sql.NullInt64 {
	return sql.NullInt64{Int64: i, Valid: i != 0}
}

// EnsurePhase5Indexes creates the composite indexes added in Phase 5 for
// campaign/agent filtering and call-log analytics. Statements are idempotent
// and errors are swallowed when an index already exists.
func (d *DB) EnsurePhase5Indexes() error {
	indexes := []struct {
		name string
		sql  string
	}{
		{"idx_leads_org_campaign_status", "CREATE INDEX idx_leads_org_campaign_status ON leads (org_id, campaign_id, status)"},
		{"idx_leads_org_executive_status", "CREATE INDEX idx_leads_org_executive_status ON leads (org_id, executive_id, status)"},
		{"idx_call_logs_campaign_created", "CREATE INDEX idx_call_logs_campaign_created ON call_logs (campaign_id, created_at)"},
		{"idx_users_org_role", "CREATE INDEX idx_users_org_role ON users (org_id, role)"},
	}
	for _, idx := range indexes {
		if _, err := d.pool.Exec(idx.sql); err != nil {
			// MySQL returns 1061 for duplicate index; ignore it.
			if !strings.Contains(err.Error(), "Duplicate") && !strings.Contains(err.Error(), "1061") {
				return fmt.Errorf("%s: %w", idx.name, err)
			}
		}
	}
	return nil
}
