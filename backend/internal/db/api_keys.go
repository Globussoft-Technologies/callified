package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
)

// APIKey mirrors the api_keys table (never exposes the raw key).
type APIKey struct {
	ID        int64  `json:"id"`
	OrgID     int64  `json:"org_id"`
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix"` // first 10 chars of the raw key for display
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

// GenerateAPIKey creates a cryptographically random API key.
// Returns (rawKey, sha256Hash, error). Store only the hash; return raw to the user once.
func GenerateAPIKey() (raw, hashed string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = fmt.Sprintf("ck_%x", b)
	sum := sha256.Sum256([]byte(raw))
	hashed = fmt.Sprintf("%x", sum)
	return
}

// CreateAPIKey inserts a new API key row (stores hash, not the raw key).
func (d *DB) CreateAPIKey(orgID int64, name, keyHash, keyPrefix string) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO api_keys (org_id, name, key_hash, key_prefix) VALUES (?,?,?,?)`,
		orgID, name, keyHash, keyPrefix)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAPIKeysByOrg returns all API keys for an org (never exposes hashes).
func (d *DB) GetAPIKeysByOrg(orgID int64) ([]APIKey, error) {
	rows, err := d.pool.Query(`
		SELECT id, org_id, name, COALESCE(key_prefix,''), COALESCE(is_active,1),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM api_keys WHERE org_id=? ORDER BY id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &k.IsActive, &k.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	return list, rows.Err()
}

// DeleteAPIKey removes an API key (scoped to org). Returns true if deleted.
func (d *DB) DeleteAPIKey(orgID, id int64) (bool, error) {
	res, err := d.pool.Exec(`DELETE FROM api_keys WHERE id=? AND org_id=?`, id, orgID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetAPIKeyByHash looks up a key by its SHA-256 hash (for inbound API auth).
// Returns (nil, nil) when no row matches so callers can distinguish "unknown
// key" (→ 401) from "key revoked" (→ 403) by inspecting IsActive on the hit.
func (d *DB) GetAPIKeyByHash(keyHash string) (*APIKey, error) {
	row := d.pool.QueryRow(`
		SELECT id, org_id, name, COALESCE(key_prefix,''), COALESCE(is_active,1),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM api_keys WHERE key_hash=?`, keyHash)
	k := &APIKey{}
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.KeyPrefix, &k.IsActive, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return k, err
}

// SetAPIKeyActive toggles is_active for a key, scoped to the caller's org so
// one org can't poke another's keys. Returns true if a row was updated.
// active=false is the "revoke" path; setting active=true reactivates a
// previously-revoked key without losing the audit trail.
func (d *DB) SetAPIKeyActive(orgID, id int64, active bool) (bool, error) {
	res, err := d.pool.Exec(
		`UPDATE api_keys SET is_active=? WHERE id=? AND org_id=?`,
		active, id, orgID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TouchAPIKey bumps last_used_at on a successful authenticated request.
// Best-effort — callers should fire-and-forget; a write failure must not
// block the inbound request.
func (d *DB) TouchAPIKey(id int64) error {
	_, err := d.pool.Exec(`UPDATE api_keys SET last_used_at=NOW() WHERE id=?`, id)
	return err
}
