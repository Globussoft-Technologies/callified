package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// CRMIntegration mirrors the crm_integrations table.
//
// SyncStatus is a derived value that lets the UI show an honest health badge
// without recomputing the rule per consumer. Issue #73 — the page rendered
// "Active Sync" (green) for a row whose Last Synced was "Never", because the
// only field exposed was is_active. The new value distinguishes:
//   - "pending"   — connected, no sync has ever run yet
//   - "active"    — synced within the last 24 hours
//   - "stale"     — last_synced_at is older than 24 hours
//   - "disabled"  — is_active=false (kept for completeness; the listing
//                   currently filters those out, so the value is mostly
//                   informational for the single-record paths.)
type CRMIntegration struct {
	ID           int64             `json:"id"`
	OrgID        int64             `json:"org_id"`
	Provider     string            `json:"provider"` // pipedrive, hubspot, salesforce, zoho
	Credentials  map[string]string `json:"credentials"`
	IsActive     bool              `json:"is_active"`
	LastSyncedAt string            `json:"last_synced_at"`
	SyncStatus   string            `json:"sync_status"`
	CreatedAt    string            `json:"created_at"`
}

// GetActiveCRMIntegrations returns all active CRM integrations across all orgs.
func (d *DB) GetActiveCRMIntegrations() ([]CRMIntegration, error) {
	rows, err := d.pool.Query(`
		SELECT id, org_id, provider, COALESCE(credentials,'{}'),
		COALESCE(is_active,1),
		COALESCE(DATE_FORMAT(last_synced_at,'%Y-%m-%d %H:%i:%s'),''),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM crm_integrations WHERE is_active=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CRMIntegration
	for rows.Next() {
		var ci CRMIntegration
		var credsJSON string
		var active int
		if err := rows.Scan(&ci.ID, &ci.OrgID, &ci.Provider, &credsJSON,
			&active, &ci.LastSyncedAt, &ci.CreatedAt); err != nil {
			return nil, err
		}
		ci.IsActive = active == 1
		ci.SyncStatus = computeSyncStatus(ci.IsActive, ci.LastSyncedAt)
		json.Unmarshal([]byte(credsJSON), &ci.Credentials) //nolint:errcheck
		list = append(list, ci)
	}
	return list, rows.Err()
}

// computeSyncStatus derives the badge state from is_active + last_synced_at.
// "active" requires a sync within the last 24h — anything older is "stale".
func computeSyncStatus(isActive bool, lastSyncedAt string) string {
	if !isActive {
		return "disabled"
	}
	if lastSyncedAt == "" {
		return "pending"
	}
	t, err := time.Parse("2006-01-02 15:04:05", lastSyncedAt)
	if err != nil {
		// Unparseable timestamp — be conservative.
		return "stale"
	}
	if time.Since(t) > 24*time.Hour {
		return "stale"
	}
	return "active"
}

// SaveCRMIntegration upserts a CRM integration for an org+provider pair.
func (d *DB) SaveCRMIntegration(orgID int64, provider string, creds map[string]string) (int64, error) {
	credsJSON, _ := json.Marshal(creds)
	res, err := d.pool.Exec(`
		INSERT INTO crm_integrations (org_id, provider, credentials, is_active)
		VALUES (?,?,?,1)
		ON DUPLICATE KEY UPDATE credentials=VALUES(credentials), is_active=1`,
		orgID, provider, string(credsJSON))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// DeleteCRMIntegration deactivates a CRM integration.
func (d *DB) DeleteCRMIntegration(orgID int64, id int64) error {
	_, err := d.pool.Exec(
		`UPDATE crm_integrations SET is_active=0 WHERE id=? AND org_id=?`, id, orgID)
	return err
}

// UpdateCRMLastSynced sets last_synced_at to now.
func (d *DB) UpdateCRMLastSynced(integrationID int64) error {
	_, err := d.pool.Exec(
		`UPDATE crm_integrations SET last_synced_at=? WHERE id=?`,
		time.Now(), integrationID)
	return err
}

// GetLeadByExternalID finds a lead by external_id + crm_provider + org.
// Returns nil when not found.
func (d *DB) GetLeadByExternalID(externalID, provider string, orgID int64) (*Lead, error) {
	row := d.pool.QueryRow(
		`SELECT `+leadCols+` FROM leads
		 WHERE external_id=? AND crm_provider=? AND org_id=?`,
		externalID, provider, orgID)
	l, err := scanLead(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return l, err
}
