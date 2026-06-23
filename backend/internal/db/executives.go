package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// EnsureExecutivesTable creates the executives table and the campaign_executives
// link table. It also adds executive_id to leads when missing.
func (d *DB) EnsureExecutivesTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS executives (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			org_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) DEFAULT '',
			phone VARCHAR(50) DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_org_id (org_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	if err != nil {
		return err
	}
	_, err = d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS campaign_executives (
			campaign_id BIGINT NOT NULL,
			executive_id BIGINT NOT NULL,
			PRIMARY KEY (campaign_id, executive_id),
			INDEX idx_campaign (campaign_id),
			INDEX idx_executive (executive_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	if err != nil {
		return err
	}
	_, _ = d.pool.Exec(`ALTER TABLE leads ADD COLUMN executive_id BIGINT DEFAULT NULL`)
	_, _ = d.pool.Exec(`ALTER TABLE leads ADD INDEX idx_executive_id (executive_id)`)
	return nil
}

// Executive is a sales/ops person managed under an org.
type Executive struct {
	ID        int64  `json:"id"`
	OrgID     int64  `json:"org_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	CreatedAt string `json:"created_at"`
}

// CreateExecutive inserts a new executive and returns the ID.
func (d *DB) CreateExecutive(orgID int64, name, email, phone string) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO executives (org_id, name, email, phone) VALUES (?,?,?,?)`,
		orgID, strings.TrimSpace(name), strings.TrimSpace(email), strings.TrimSpace(phone))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateExecutive updates an executive scoped to the org.
func (d *DB) UpdateExecutive(id, orgID int64, name, email, phone string) error {
	_, err := d.pool.Exec(
		`UPDATE executives SET name=?, email=?, phone=? WHERE id=? AND org_id=?`,
		strings.TrimSpace(name), strings.TrimSpace(email), strings.TrimSpace(phone), id, orgID)
	return err
}

// DeleteExecutive removes an executive scoped to the org.
func (d *DB) DeleteExecutive(id, orgID int64) error {
	_, err := d.pool.Exec(`DELETE FROM executives WHERE id=? AND org_id=?`, id, orgID)
	return err
}

// UnassignExecutiveFromLeads sets executive_id=NULL for leads in the org that
// reference the given executive.
func (d *DB) UnassignExecutiveFromLeads(executiveID, orgID int64) error {
	_, err := d.pool.Exec(
		`UPDATE leads SET executive_id=NULL WHERE executive_id=? AND (org_id=? OR org_id IS NULL)`,
		executiveID, orgID)
	return err
}

// GetExecutivesByOrg returns all executives for an org.
func (d *DB) GetExecutivesByOrg(orgID int64) ([]Executive, error) {
	rows, err := d.pool.Query(
		`SELECT id, org_id, name, email, phone, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		 FROM executives WHERE org_id=? ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Executive
	for rows.Next() {
		var e Executive
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Name, &e.Email, &e.Phone, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// GetExecutiveByID fetches one executive scoped to org.
func (d *DB) GetExecutiveByID(id, orgID int64) (*Executive, error) {
	var e Executive
	err := d.pool.QueryRow(
		`SELECT id, org_id, name, email, phone, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		 FROM executives WHERE id=? AND org_id=?`, id, orgID).
		Scan(&e.ID, &e.OrgID, &e.Name, &e.Email, &e.Phone, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}

// SetCampaignExecutives replaces the executive assignments for a campaign.
func (d *DB) SetCampaignExecutives(campaignID int64, execIDs []int64) error {
	tx, err := d.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM campaign_executives WHERE campaign_id=?`, campaignID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO campaign_executives (campaign_id, executive_id) VALUES (?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, eid := range execIDs {
		if eid <= 0 {
			continue
		}
		if _, err := stmt.Exec(campaignID, eid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetCampaignExecutiveIDs returns the executive IDs assigned to a campaign.
func (d *DB) GetCampaignExecutiveIDs(campaignID int64) ([]int64, error) {
	rows, err := d.pool.Query(
		`SELECT executive_id FROM campaign_executives WHERE campaign_id=? ORDER BY executive_id`, campaignID)
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

// UpdateLeadExecutive assigns or unassigns (execID=0) an executive to a lead.
func (d *DB) UpdateLeadExecutive(id, orgID, execID int64) error {
	var exec interface{} = nil
	if execID > 0 {
		exec = execID
	}
	res, err := d.pool.Exec(
		`UPDATE leads SET executive_id=? WHERE id=? AND (org_id=? OR org_id IS NULL)`,
		exec, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("lead not found")
	}
	return nil
}
