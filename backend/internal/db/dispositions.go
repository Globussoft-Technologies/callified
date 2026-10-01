package db

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var dispositionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

var defaultDispositionOptions = []DispositionOption{
	{Code: "interested", Label: "Interested", Color: "#16a34a", SortOrder: 10},
	{Code: "not_interested", Label: "Not Interested", Color: "#dc2626", SortOrder: 20},
	{Code: "callback", Label: "Callback", Color: "#2563eb", SortOrder: 30},
	{Code: "appointment_booked", Label: "Appointment Booked", Color: "#059669", SortOrder: 40},
	{Code: "no_answer", Label: "No Answer", Color: "#d97706", SortOrder: 50},
	{Code: "wrong_number", Label: "Wrong Number", Color: "#7c3aed", SortOrder: 60},
	{Code: "dnc", Label: "Do Not Call", Color: "#b91c1c", SortOrder: 70},
}

type DispositionOption struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	CampaignID int64  `json:"campaign_id"`
	Code       string `json:"code"`
	Label      string `json:"label"`
	Color      string `json:"color"`
	SortOrder  int    `json:"sort_order"`
	IsActive   bool   `json:"is_active"`
}

type CallDisposition struct {
	ID                   int64   `json:"id"`
	OrgID                int64   `json:"org_id"`
	CampaignID           int64   `json:"campaign_id"`
	LeadID               int64   `json:"lead_id"`
	CallLogID            int64   `json:"call_log_id"`
	TranscriptID         int64   `json:"transcript_id"`
	CallSid              string  `json:"call_sid"`
	AgentUserID          int64   `json:"agent_user_id"`
	AIDispositionCode    string  `json:"ai_disposition_code"`
	AISummary            string  `json:"ai_summary"`
	AISentiment          string  `json:"ai_sentiment"`
	AIConfidence         float64 `json:"ai_confidence"`
	AIObjections         string  `json:"ai_objections"`
	AINextAction         string  `json:"ai_next_action"`
	FinalDispositionCode string  `json:"final_disposition_code"`
	EditedSummary        string  `json:"edited_summary"`
	OverrideReason       string  `json:"override_reason"`
	OverriddenBy         int64   `json:"overridden_by"`
	OverriddenAt         string  `json:"overridden_at"`
	AnalyzedAt           string  `json:"analyzed_at"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
	EffectiveCode        string  `json:"effective_code"`
	EffectiveSummary     string  `json:"effective_summary"`
	Source               string  `json:"source"`
}

type AICallDisposition struct {
	OrgID        int64
	CampaignID   int64
	LeadID       int64
	CallLogID    int64
	TranscriptID int64
	CallSid      string
	AgentUserID  int64
	Code         string
	Summary      string
	Sentiment    string
	Confidence   float64
	Objections   string
	NextAction   string
}

func (d *DB) EnsureDispositionTables() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS campaign_disposition_options (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, org_id BIGINT NOT NULL, campaign_id BIGINT NOT NULL,
			code VARCHAR(64) NOT NULL, label VARCHAR(100) NOT NULL, color VARCHAR(20) NOT NULL DEFAULT '#64748b',
			sort_order INT NOT NULL DEFAULT 0, is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uq_campaign_disposition_code (campaign_id, code),
			KEY idx_campaign_disposition_org (org_id, campaign_id, is_active)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS call_dispositions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, org_id BIGINT NOT NULL, campaign_id BIGINT DEFAULT NULL,
			lead_id BIGINT DEFAULT NULL, call_log_id BIGINT DEFAULT NULL, transcript_id BIGINT DEFAULT NULL,
			call_sid VARCHAR(255) DEFAULT NULL, agent_user_id BIGINT DEFAULT NULL,
			ai_disposition_code VARCHAR(64) NOT NULL DEFAULT 'callback', ai_summary TEXT,
			ai_sentiment VARCHAR(32) NOT NULL DEFAULT 'neutral', ai_confidence DECIMAL(5,4) NOT NULL DEFAULT 0,
			ai_objections TEXT, ai_next_action TEXT, final_disposition_code VARCHAR(64) DEFAULT NULL,
			edited_summary TEXT, override_reason TEXT, overridden_by BIGINT DEFAULT NULL,
			overridden_at TIMESTAMP NULL DEFAULT NULL, analyzed_at TIMESTAMP NULL DEFAULT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uq_call_disposition_log (call_log_id), UNIQUE KEY uq_call_disposition_transcript (transcript_id),
			KEY idx_call_disposition_lead (org_id, lead_id, created_at),
			KEY idx_call_disposition_campaign (org_id, campaign_id, created_at), KEY idx_call_disposition_sid (call_sid))
			ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS call_disposition_events (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, disposition_id BIGINT NOT NULL, org_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL, previous_code VARCHAR(64) DEFAULT NULL, new_code VARCHAR(64) NOT NULL,
			previous_summary TEXT, new_summary TEXT, reason TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_disposition_events_record (disposition_id, created_at),
			KEY idx_disposition_events_org (org_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, statement := range statements {
		if _, err := d.pool.Exec(statement); err != nil {
			return err
		}
	}
	if _, err := d.pool.Exec(`ALTER TABLE call_logs ADD COLUMN agent_user_id BIGINT DEFAULT NULL`); err != nil && !isMySQLError(err, 1060) {
		return fmt.Errorf("add call_logs.agent_user_id: %w", err)
	}
	return nil
}

func (d *DB) ensureDefaultDispositionOptions(orgID, campaignID int64) error {
	if orgID <= 0 || campaignID <= 0 {
		return nil
	}
	for _, option := range defaultDispositionOptions {
		_, err := d.pool.Exec(`INSERT IGNORE INTO campaign_disposition_options
			(org_id, campaign_id, code, label, color, sort_order, is_active) VALUES (?,?,?,?,?,?,1)`,
			orgID, campaignID, option.Code, option.Label, option.Color, option.SortOrder)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) GetCampaignDispositionOptions(orgID, campaignID int64) ([]DispositionOption, error) {
	if err := d.ensureDefaultDispositionOptions(orgID, campaignID); err != nil {
		return nil, err
	}
	rows, err := d.pool.Query(`SELECT id, org_id, campaign_id, code, label, color, sort_order, is_active
		FROM campaign_disposition_options WHERE org_id=? AND campaign_id=? ORDER BY sort_order, id`, orgID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []DispositionOption
	for rows.Next() {
		var option DispositionOption
		if err := rows.Scan(&option.ID, &option.OrgID, &option.CampaignID, &option.Code, &option.Label,
			&option.Color, &option.SortOrder, &option.IsActive); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func normalizeDispositionOption(option DispositionOption, index int) (DispositionOption, error) {
	option.Code = strings.ToLower(strings.TrimSpace(option.Code))
	option.Label = strings.TrimSpace(option.Label)
	option.Color = strings.TrimSpace(option.Color)
	if !dispositionCodePattern.MatchString(option.Code) {
		return option, fmt.Errorf("invalid disposition code %q", option.Code)
	}
	if option.Label == "" || len(option.Label) > 100 {
		return option, fmt.Errorf("invalid label for %s", option.Code)
	}
	if option.Color == "" {
		option.Color = "#64748b"
	}
	if option.SortOrder == 0 {
		option.SortOrder = (index + 1) * 10
	}
	option.IsActive = true
	return option, nil
}

func (d *DB) ReplaceCampaignDispositionOptions(orgID, campaignID int64, options []DispositionOption) error {
	if len(options) == 0 {
		return fmt.Errorf("at least one disposition option is required")
	}
	normalized := make([]DispositionOption, 0, len(options))
	seen := map[string]bool{}
	for index, option := range options {
		option, err := normalizeDispositionOption(option, index)
		if err != nil {
			return err
		}
		if seen[option.Code] {
			return fmt.Errorf("duplicate disposition code %q", option.Code)
		}
		seen[option.Code] = true
		normalized = append(normalized, option)
	}
	sort.SliceStable(normalized, func(i, j int) bool { return normalized[i].SortOrder < normalized[j].SortOrder })

	tx, err := d.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE campaign_disposition_options SET is_active=0 WHERE org_id=? AND campaign_id=?`, orgID, campaignID); err != nil {
		return err
	}
	for _, option := range normalized {
		if _, err := tx.Exec(`INSERT INTO campaign_disposition_options
			(org_id, campaign_id, code, label, color, sort_order, is_active) VALUES (?,?,?,?,?,?,1)
			ON DUPLICATE KEY UPDATE label=VALUES(label), color=VALUES(color), sort_order=VALUES(sort_order), is_active=1`,
			orgID, campaignID, option.Code, option.Label, option.Color, option.SortOrder); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) SaveAICallDisposition(value AICallDisposition) error {
	value.Code = strings.ToLower(strings.TrimSpace(value.Code))
	if value.Code == "" {
		value.Code = "callback"
	}
	if value.Confidence < 0 {
		value.Confidence = 0
	}
	if value.Confidence > 1 {
		value.Confidence = 1
	}
	_, err := d.pool.Exec(`INSERT INTO call_dispositions
		(org_id, campaign_id, lead_id, call_log_id, transcript_id, call_sid, agent_user_id,
		 ai_disposition_code, ai_summary, ai_sentiment, ai_confidence, ai_objections, ai_next_action, analyzed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,NOW())
		ON DUPLICATE KEY UPDATE org_id=VALUES(org_id), campaign_id=VALUES(campaign_id), lead_id=VALUES(lead_id),
		call_log_id=COALESCE(call_log_id, VALUES(call_log_id)), transcript_id=COALESCE(transcript_id, VALUES(transcript_id)),
		call_sid=COALESCE(NULLIF(call_sid,''), VALUES(call_sid)),
		agent_user_id=COALESCE(agent_user_id, VALUES(agent_user_id)), ai_disposition_code=VALUES(ai_disposition_code),
		ai_summary=VALUES(ai_summary), ai_sentiment=VALUES(ai_sentiment), ai_confidence=VALUES(ai_confidence),
		ai_objections=VALUES(ai_objections), ai_next_action=VALUES(ai_next_action), analyzed_at=NOW()`,
		value.OrgID, nullInt64(value.CampaignID), nullInt64(value.LeadID), nullInt64(value.CallLogID),
		nullInt64(value.TranscriptID), nullString(value.CallSid), nullInt64(value.AgentUserID), value.Code,
		nullString(value.Summary), nullString(value.Sentiment), value.Confidence, nullString(value.Objections), nullString(value.NextAction))
	return err
}

func scanCallDisposition(row interface{ Scan(...any) error }) (*CallDisposition, error) {
	value := &CallDisposition{}
	err := row.Scan(&value.ID, &value.OrgID, &value.CampaignID, &value.LeadID, &value.CallLogID,
		&value.TranscriptID, &value.CallSid, &value.AgentUserID, &value.AIDispositionCode,
		&value.AISummary, &value.AISentiment, &value.AIConfidence, &value.AIObjections,
		&value.AINextAction, &value.FinalDispositionCode, &value.EditedSummary,
		&value.OverrideReason, &value.OverriddenBy, &value.OverriddenAt, &value.AnalyzedAt,
		&value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	value.EffectiveCode = value.AIDispositionCode
	value.EffectiveSummary = value.AISummary
	value.Source = "ai"
	if value.FinalDispositionCode != "" {
		value.EffectiveCode = value.FinalDispositionCode
		value.Source = "human"
	}
	if value.EditedSummary != "" {
		value.EffectiveSummary = value.EditedSummary
	}
	return value, nil
}

const callDispositionColumns = `id, org_id, COALESCE(campaign_id,0), COALESCE(lead_id,0),
	COALESCE(call_log_id,0), COALESCE(transcript_id,0), COALESCE(call_sid,''), COALESCE(agent_user_id,0),
	COALESCE(ai_disposition_code,'callback'), COALESCE(ai_summary,''), COALESCE(ai_sentiment,'neutral'),
	COALESCE(ai_confidence,0), COALESCE(ai_objections,''), COALESCE(ai_next_action,''),
	COALESCE(final_disposition_code,''), COALESCE(edited_summary,''), COALESCE(override_reason,''),
	COALESCE(overridden_by,0), COALESCE(DATE_FORMAT(overridden_at,'%Y-%m-%d %H:%i:%s'),''),
	COALESCE(DATE_FORMAT(analyzed_at,'%Y-%m-%d %H:%i:%s'),''),
	DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'), DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')`

func (d *DB) GetCallDispositionByTranscript(orgID, transcriptID int64) (*CallDisposition, error) {
	return scanCallDisposition(d.pool.QueryRow(`SELECT `+callDispositionColumns+`
		FROM call_dispositions WHERE org_id=? AND transcript_id=?`, orgID, transcriptID))
}

func (d *DB) OverrideCallDisposition(orgID, transcriptID, userID int64, code, summary, reason string) (*CallDisposition, error) {
	current, err := d.GetCallDispositionByTranscript(orgID, transcriptID)
	if err != nil || current == nil {
		return current, err
	}
	code = strings.ToLower(strings.TrimSpace(code))
	var exists int
	if err := d.pool.QueryRow(`SELECT COUNT(*) FROM campaign_disposition_options
		WHERE org_id=? AND campaign_id=? AND code=? AND is_active=1`, orgID, current.CampaignID, code).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, fmt.Errorf("disposition option is not active for this campaign")
	}
	tx, err := d.pool.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO call_disposition_events
		(disposition_id, org_id, user_id, previous_code, new_code, previous_summary, new_summary, reason)
		VALUES (?,?,?,?,?,?,?,?)`, current.ID, orgID, userID, current.EffectiveCode, code,
		nullString(current.EffectiveSummary), nullString(strings.TrimSpace(summary)), nullString(strings.TrimSpace(reason))); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE call_dispositions SET final_disposition_code=?, edited_summary=?, override_reason=?,
		overridden_by=?, overridden_at=NOW() WHERE id=? AND org_id=?`, code, nullString(strings.TrimSpace(summary)),
		nullString(strings.TrimSpace(reason)), userID, current.ID, orgID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetCallDispositionByTranscript(orgID, transcriptID)
}

func (d *DB) GetCallLogIDByCallSid(callSid string) (int64, error) {
	if strings.TrimSpace(callSid) == "" {
		return 0, nil
	}
	var id int64
	err := d.pool.QueryRow(`SELECT id FROM call_logs WHERE call_sid=? ORDER BY id DESC LIMIT 1`, callSid).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (d *DB) SaveSystemDispositionByCallSid(callSid, code, summary string) error {
	row := d.pool.QueryRow(`SELECT id, org_id, COALESCE(campaign_id,0), lead_id, COALESCE(agent_user_id,0)
		FROM call_logs WHERE call_sid=? ORDER BY id DESC LIMIT 1`, callSid)
	var callLogID, orgID, campaignID, leadID, agentUserID int64
	if err := row.Scan(&callLogID, &orgID, &campaignID, &leadID, &agentUserID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	return d.SaveAICallDisposition(AICallDisposition{OrgID: orgID, CampaignID: campaignID, LeadID: leadID,
		CallLogID: callLogID, CallSid: callSid, AgentUserID: agentUserID, Code: code, Summary: summary,
		Sentiment: "neutral", Confidence: 1, NextAction: "Review the call outcome and retry when appropriate."})
}

func DefaultDispositionOptions() []DispositionOption {
	options := make([]DispositionOption, len(defaultDispositionOptions))
	copy(options, defaultDispositionOptions)
	return options
}

// SystemDispositionForCallStatus maps terminal carrier states that do not
// require transcript analysis to a deterministic disposition.
func SystemDispositionForCallStatus(status string) (code, summary string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "no-answer", "no_answer", "unanswered":
		return "no_answer", "The customer did not answer the call.", true
	case "busy":
		return "callback", "The line was busy; retry the call later.", true
	case "failed", "cancelled", "canceled":
		return "no_answer", "The call could not be connected.", true
	default:
		return "", "", false
	}
}
