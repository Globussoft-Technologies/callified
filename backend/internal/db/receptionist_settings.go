package db

import "database/sql"

type ReceptionistSettings struct {
	Greeting     string `json:"greeting"`
	Instructions string `json:"instructions"`
}

func (d *DB) EnsureReceptionistSettingsTable() error {
	_, err := d.pool.Exec(`CREATE TABLE IF NOT EXISTS org_receptionist_settings (
 org_id BIGINT NOT NULL PRIMARY KEY,
 greeting TEXT NOT NULL,
 instructions TEXT NOT NULL,
 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
)`)
	return err
}

func (d *DB) GetReceptionistSettings(orgID int64) (ReceptionistSettings, error) {
	var settings ReceptionistSettings
	err := d.pool.QueryRow("SELECT greeting, instructions FROM org_receptionist_settings WHERE org_id = ?", orgID).Scan(&settings.Greeting, &settings.Instructions)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	return settings, err
}

func (d *DB) SaveReceptionistSettings(orgID int64, settings ReceptionistSettings) error {
	_, err := d.pool.Exec(`INSERT INTO org_receptionist_settings (org_id, greeting, instructions) VALUES (?, ?, ?)
 ON DUPLICATE KEY UPDATE greeting=VALUES(greeting), instructions=VALUES(instructions)`, orgID, settings.Greeting, settings.Instructions)
	return err
}
