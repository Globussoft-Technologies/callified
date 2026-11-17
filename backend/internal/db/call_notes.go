package db

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// IsManualCall distinguishes human-agent transcripts from AI conversations.
func (t *Transcript) IsManualCall() bool {
	var turns []struct {
		Role    string `json:"role"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if json.Unmarshal(t.Transcript, &turns) != nil {
		return false
	}
	for _, turn := range turns {
		if strings.EqualFold(turn.Role, "agent") || (strings.EqualFold(turn.Role, "system") && strings.Contains(turn.Text+turn.Content, "Human call")) {
			return true
		}
	}
	return false
}

func (d *DB) GetManualCallNotes(orgID, transcriptID int64) (string, error) {
	var notes string
	err := d.pool.QueryRow(`SELECT notes FROM manual_call_notes WHERE org_id=? AND transcript_id=?`, orgID, transcriptID).Scan(&notes)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return notes, err
}

func (d *DB) SaveManualCallNotes(orgID, transcriptID, userID int64, notes string) error {
	_, err := d.pool.Exec(`INSERT INTO manual_call_notes (org_id,transcript_id,notes,updated_by) VALUES (?,?,?,?)
 ON DUPLICATE KEY UPDATE notes=VALUES(notes),updated_by=VALUES(updated_by),updated_at=CURRENT_TIMESTAMP`, orgID, transcriptID, notes, userID)
	return err
}
