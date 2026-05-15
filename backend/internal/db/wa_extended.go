package db

// WAConversationRow is a row from whatsapp_conversations joined with lead info.
// IsMuted suppresses the AI auto-reply on this thread without affecting other
// threads on the same channel; IsArchived hides the row from the default
// inbox view (still listable via the "show archived" toggle so the operator
// can recover them if archived by mistake).
type WAConversationRow struct {
	ID           int64  `json:"id"`
	OrgID        int64  `json:"org_id"`
	Phone        string `json:"phone"`
	Provider     string `json:"provider"`
	LastMessage  string `json:"last_message"`
	MessageCount int    `json:"message_count"`
	LeadID       int64  `json:"lead_id"`
	LeadName     string `json:"lead_name"`
	IsMuted      bool   `json:"is_muted"`
	IsArchived   bool   `json:"is_archived"`
	UpdatedAt    string `json:"updated_at"`
}

// GetAllWhatsappLogs returns recent outbound/inbound WA conversation rows for an org.
// Includes archived rows so the audit log isn't silently shorter than the
// inbox a user thinks they archived. UI is responsible for filtering.
func (d *DB) GetAllWhatsappLogs(orgID int64) ([]WAConversationRow, error) {
	rows, err := d.pool.Query(`
		SELECT c.id, c.org_id, c.phone, COALESCE(c.provider,''),
		COALESCE(c.last_message,''), COALESCE(c.message_count,0),
		COALESCE(c.lead_id,0), COALESCE(l.first_name,''),
		COALESCE(c.is_muted,0), COALESCE(c.is_archived,0),
		DATE_FORMAT(c.updated_at,'%Y-%m-%d %H:%i:%s')
		FROM whatsapp_conversations c
		LEFT JOIN leads l ON c.lead_id=l.id
		WHERE c.org_id=?
		ORDER BY c.updated_at DESC
		LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []WAConversationRow
	for rows.Next() {
		var row WAConversationRow
		var muted, archived int
		if err := rows.Scan(&row.ID, &row.OrgID, &row.Phone, &row.Provider,
			&row.LastMessage, &row.MessageCount, &row.LeadID, &row.LeadName,
			&muted, &archived,
			&row.UpdatedAt); err != nil {
			return nil, err
		}
		row.IsMuted = muted == 1
		row.IsArchived = archived == 1
		list = append(list, row)
	}
	return list, rows.Err()
}

// ─── Conversation management (mute / archive / clear / delete) ───
//
// All four are scoped to (org_id, phone) so a malicious / buggy client
// can't reach into another org's conversations by guessing phone IDs.
// Mute and Archive are idempotent toggles; Clear and Delete are
// destructive (no row resurrection). The handlers in api/wa_config.go
// gate destructive ones behind a themed confirm modal in the UI.

// IsWAConversationAIEnabled reports whether the dashboard's per-thread
// "AI Auto-Reply" toggle is ON for this conversation. Defaults to TRUE
// when the row doesn't exist yet — that matches the dashboard's default
// display (toggle starts in ON state for new conversations) and means a
// brand-new inbound from an unknown number still gets an AI reply.
// Returns false only when the column is explicitly 0.
func (d *DB) IsWAConversationAIEnabled(orgID int64, phone string) bool {
	var enabled int
	err := d.pool.QueryRow(
		`SELECT COALESCE(ai_enabled,1) FROM whatsapp_conversations
		 WHERE org_id=? AND phone=? LIMIT 1`, orgID, phone).Scan(&enabled)
	if err != nil {
		// No row yet (first inbound) — treat as enabled so the AI
		// kicks in by default. Operator can flip it off after.
		return true
	}
	return enabled == 1
}

// IsWAConversationMuted reports whether a thread has the mute flag set.
// Used by the inbound webhook to skip AI auto-reply on muted threads
// without breaking the rest of the pipeline (message still gets saved).
// Returns false on lookup failure rather than erroring — it's better to
// auto-reply on a thread that should have been muted than to drop the
// reply on a transient DB blip.
func (d *DB) IsWAConversationMuted(orgID int64, phone string) bool {
	var muted int
	err := d.pool.QueryRow(
		`SELECT COALESCE(is_muted,0) FROM whatsapp_conversations
		 WHERE org_id=? AND phone=? LIMIT 1`, orgID, phone).Scan(&muted)
	if err != nil {
		return false
	}
	return muted == 1
}

// SetWAConversationMuted flips is_muted on one conversation. When muted,
// the AI auto-reply branch in the webhook handler skips this thread but
// still saves the inbound message — the operator can take over manually.
func (d *DB) SetWAConversationMuted(orgID int64, phone string, muted bool) error {
	v := 0
	if muted {
		v = 1
	}
	_, err := d.pool.Exec(
		`UPDATE whatsapp_conversations SET is_muted=?
		 WHERE org_id=? AND phone=?`, v, orgID, phone)
	return err
}

// SetWAConversationArchived flips is_archived. Archived rows are hidden
// from the default inbox listing; the dashboard's "Show archived" toggle
// flips a query param that allows them through.
func (d *DB) SetWAConversationArchived(orgID int64, phone string, archived bool) error {
	v := 0
	if archived {
		v = 1
	}
	_, err := d.pool.Exec(
		`UPDATE whatsapp_conversations SET is_archived=?
		 WHERE org_id=? AND phone=?`, v, orgID, phone)
	return err
}

// ClearWAConversationMessages wipes the message history for a phone but
// keeps the conversation row (and its mute/archive flags) intact, so the
// thread continues to exist but starts empty.
func (d *DB) ClearWAConversationMessages(orgID int64, phone string) error {
	// Two-step delete: scope the message-delete by joining through the
	// org-owned conversation row so a phone collision across orgs (rare,
	// but theoretically possible) can't wipe another tenant's data.
	res, err := d.pool.Exec(`
		DELETE m FROM whatsapp_messages m
		JOIN whatsapp_conversations c ON c.id = m.conversation_id
		WHERE c.org_id=? AND c.phone=?`, orgID, phone)
	if err != nil {
		return err
	}
	// Reset the running counters on the conversation so the inbox preview
	// shows "no messages" instead of stale data.
	if _, err := d.pool.Exec(`
		UPDATE whatsapp_conversations
		SET last_message='', message_count=0, updated_at=NOW()
		WHERE org_id=? AND phone=?`, orgID, phone); err != nil {
		return err
	}
	_ = res
	return nil
}

// DeleteWAConversation removes a conversation entirely (messages + row).
// Irreversible — the API handler wraps this behind a themed confirm.
func (d *DB) DeleteWAConversation(orgID int64, phone string) error {
	if _, err := d.pool.Exec(`
		DELETE m FROM whatsapp_messages m
		JOIN whatsapp_conversations c ON c.id = m.conversation_id
		WHERE c.org_id=? AND c.phone=?`, orgID, phone); err != nil {
		return err
	}
	if _, err := d.pool.Exec(
		`DELETE FROM whatsapp_conversations WHERE org_id=? AND phone=?`,
		orgID, phone); err != nil {
		return err
	}
	return nil
}
