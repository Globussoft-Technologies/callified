package db

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// CallReport is the enriched payload for GET /api/calls/{call_id}/report.
type CallReport struct {
	TranscriptID   int64           `json:"transcript_id"`
	LeadID         int64           `json:"lead_id"`
	CampaignID     int64           `json:"campaign_id"`
	OrgID          int64           `json:"org_id"`
	LeadFirstName  string          `json:"lead_first_name"`
	LeadLastName   string          `json:"lead_last_name"`
	LeadPhone      string          `json:"lead_phone"`
	CampaignName   string          `json:"campaign_name"`
	Transcript     json.RawMessage `json:"transcript"`
	RecordingURL   string          `json:"recording_url"`
	TTSLanguage    string          `json:"tts_language"`
	CallDurationS  float64         `json:"call_duration_s"`
	CostEstimate   float64         `json:"cost_estimate_paise"`
	Review         *CallReview     `json:"review,omitempty"`
	AnalysisPending bool           `json:"analysis_pending"`
	CreatedAt      string          `json:"created_at"`
}

// GetCallReport returns a transcript with lead/campaign context and any existing review.
// It does NOT trigger analysis; that is handled by the API layer.
func (d *DB) GetCallReport(transcriptID int64) (*CallReport, error) {
	row := d.pool.QueryRow(`
		SELECT ct.id, COALESCE(ct.lead_id,0), COALESCE(ct.campaign_id,0), COALESCE(ct.org_id,0),
		       COALESCE(l.first_name,''), COALESCE(l.last_name,''), COALESCE(l.phone,''),
		       COALESCE(c.name,''),
		       COALESCE(ct.transcript,'[]'), COALESCE(ct.recording_url,''),
		       COALESCE(ct.tts_language,''), COALESCE(ct.call_duration_s,0),
		       DATE_FORMAT(ct.created_at,'%Y-%m-%d %H:%i:%s')
		FROM call_transcripts ct
		LEFT JOIN leads l ON l.id=ct.lead_id
		LEFT JOIN campaigns c ON c.id=ct.campaign_id
		WHERE ct.id=?`, transcriptID)
	var r CallReport
	err := row.Scan(&r.TranscriptID, &r.LeadID, &r.CampaignID, &r.OrgID,
		&r.LeadFirstName, &r.LeadLastName, &r.LeadPhone,
		&r.CampaignName,
		&r.Transcript, &r.RecordingURL, &r.TTSLanguage, &r.CallDurationS, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CostEstimate = estimateCallCost(r.TTSLanguage, r.CallDurationS)
	return &r, nil
}

// estimateCallCost returns a platform-standard cost estimate in paise.
// Indic languages cost 3 paise/sec; English costs 2 paise/sec.
func estimateCallCost(language string, durationS float64) float64 {
	rate := 2.0 // paise per second for English
	if isIndicLanguage(language) {
		rate = 3.0
	}
	return rate * durationS
}

func isIndicLanguage(language string) bool {
	switch language {
	case "hi", "mr", "bn", "gu", "pa", "ta", "te", "kn", "ml":
		return true
	}
	return false
}
