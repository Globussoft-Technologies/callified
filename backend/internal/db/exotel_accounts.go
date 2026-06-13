package db

import (
	"database/sql"
	"errors"
)

// EnsureOrgExotelAccountsTable creates the org_exotel_accounts table if it doesn't exist.
func (d *DB) EnsureOrgExotelAccountsTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS org_exotel_accounts (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			org_id BIGINT NOT NULL,
			provider VARCHAR(50) DEFAULT 'exotel',
			name VARCHAR(255) NOT NULL,
			api_key VARCHAR(512) NOT NULL,
			api_token VARCHAR(512) NOT NULL,
			api_secret VARCHAR(512) DEFAULT '',
			account_sid VARCHAR(255) NOT NULL,
			caller_id VARCHAR(50) NOT NULL,
			app_id VARCHAR(255) DEFAULT '',
			app_type VARCHAR(20) DEFAULT 'exoml',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_org_id (org_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	if err != nil {
		return err
	}
	// Backward-compat: existing rows created before this column was added
	// default to the legacy ExoML XML behaviour.
	_, _ = d.pool.Exec(`ALTER TABLE org_exotel_accounts ADD COLUMN app_type VARCHAR(20) DEFAULT 'exoml'`)
	return nil
}

// OrgExotelAccount holds a named set of provider credentials (Exotel or Twilio)
// stored at the org level. Multiple accounts let a single org run campaigns
// with different providers / sub-accounts.
type OrgExotelAccount struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	Provider   string `json:"provider"` // "exotel" or "twilio"
	Name       string `json:"name"`
	APIKey     string `json:"api_key"`    // Exotel: API Key   | Twilio: Auth Token
	APIToken   string `json:"api_token"`  // Exotel: API Token | Twilio: API Key SID (SK…)
	APISecret  string `json:"api_secret"` // Twilio only: API Secret
	AccountSID string `json:"account_sid"`
	CallerID   string `json:"caller_id"` // Exotel: Caller ID | Twilio: Phone Number
	AppID      string `json:"app_id"`    // Exotel: App ID    | Twilio: TwiML App SID
	AppType    string `json:"app_type"`  // Exotel: 'exoml' (legacy XML) or 'voicebot' (AgentStream JSON)
	CreatedAt  string `json:"created_at"`
}

// GetOrgExotelAccounts returns all saved provider accounts for an org.
func (d *DB) GetOrgExotelAccounts(orgID int64) ([]OrgExotelAccount, error) {
	rows, err := d.pool.Query(`
		SELECT id, org_id, COALESCE(provider,'exotel'), name, api_key, api_token,
		       COALESCE(api_secret,''), account_sid, caller_id,
		       COALESCE(app_id,''), COALESCE(app_type,'exoml'), DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM org_exotel_accounts WHERE org_id=? ORDER BY id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []OrgExotelAccount
	for rows.Next() {
		var a OrgExotelAccount
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Provider, &a.Name, &a.APIKey, &a.APIToken,
			&a.APISecret, &a.AccountSID, &a.CallerID, &a.AppID, &a.AppType, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// GetOrgExotelAccountByID fetches a single account, scoping to orgID.
func (d *DB) GetOrgExotelAccountByID(id, orgID int64) (*OrgExotelAccount, error) {
	a := &OrgExotelAccount{}
	err := d.pool.QueryRow(`
		SELECT id, org_id, COALESCE(provider,'exotel'), name, api_key, api_token,
		       COALESCE(api_secret,''), account_sid, caller_id,
		       COALESCE(app_id,''), COALESCE(app_type,'exoml'), DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM org_exotel_accounts WHERE id=? AND org_id=?`, id, orgID).
		Scan(&a.ID, &a.OrgID, &a.Provider, &a.Name, &a.APIKey, &a.APIToken,
			&a.APISecret, &a.AccountSID, &a.CallerID, &a.AppID, &a.AppType, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// CreateOrgExotelAccount inserts a new account and returns its ID.
func (d *DB) CreateOrgExotelAccount(orgID int64, provider, name, apiKey, apiToken, apiSecret, accountSID, callerID, appID, appType string) (int64, error) {
	if appType == "" {
		appType = "exoml"
	}
	res, err := d.pool.Exec(`
		INSERT INTO org_exotel_accounts (org_id, provider, name, api_key, api_token, api_secret, account_sid, caller_id, app_id, app_type)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		orgID, provider, name, apiKey, apiToken, apiSecret, accountSID, callerID, appID, appType)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateOrgExotelAccount updates all mutable fields on an existing account.
func (d *DB) UpdateOrgExotelAccount(id, orgID int64, provider, name, apiKey, apiToken, apiSecret, accountSID, callerID, appID, appType string) error {
	if appType == "" {
		appType = "exoml"
	}
	_, err := d.pool.Exec(`
		UPDATE org_exotel_accounts
		SET provider=?, name=?, api_key=?, api_token=?, api_secret=?, account_sid=?, caller_id=?, app_id=?, app_type=?
		WHERE id=? AND org_id=?`,
		provider, name, apiKey, apiToken, apiSecret, accountSID, callerID, appID, appType, id, orgID)
	return err
}

// DeleteOrgExotelAccount removes an account, scoping the delete to orgID.
func (d *DB) DeleteOrgExotelAccount(id, orgID int64) error {
	_, err := d.pool.Exec(`DELETE FROM org_exotel_accounts WHERE id=? AND org_id=?`, id, orgID)
	return err
}

// GetCampaignExotelAccountID returns the exotel_account_id linked to a campaign (0 if none).
func (d *DB) GetCampaignExotelAccountID(campaignID int64) (int64, error) {
	var id int64
	err := d.pool.QueryRow(`SELECT COALESCE(exotel_account_id,0) FROM campaigns WHERE id=?`, campaignID).Scan(&id)
	return id, err
}

// GetExotelAppTypeByAppID returns the app_type for the first org_exotel_accounts
// row matching the given app_id/flow_id. Empty string means no match.
func (d *DB) GetExotelAppTypeByAppID(appID string) string {
	if appID == "" {
		return ""
	}
	var appType string
	_ = d.pool.QueryRow(`SELECT COALESCE(app_type,'exoml') FROM org_exotel_accounts WHERE app_id=? LIMIT 1`, appID).Scan(&appType)
	return appType
}

// GetOrgExotelAccountCreds returns an ExotelCreds from an org-level account.
// Returns zero-value ExotelCreds (IsSet()=false) when the ID is 0 or not found.
func (d *DB) GetOrgExotelAccountCreds(accountID, orgID int64) (ExotelCreds, error) {
	if accountID == 0 {
		return ExotelCreds{}, nil
	}
	a, err := d.GetOrgExotelAccountByID(accountID, orgID)
	if err != nil || a == nil {
		return ExotelCreds{}, err
	}
	return ExotelCreds{
		Provider:   a.Provider,
		APIKey:     a.APIKey,
		APIToken:   a.APIToken,
		APISecret:  a.APISecret,
		AccountSID: a.AccountSID,
		CallerID:   a.CallerID,
		AppID:      a.AppID,
		AppType:    a.AppType,
	}, nil
}
