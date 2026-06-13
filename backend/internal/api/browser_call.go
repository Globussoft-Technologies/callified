package api

import (
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/dial"
)

// browserCall initiates a browser-to-phone call for a specific campaign lead.
//
// POST /api/campaigns/{id}/leads/{lead_id}/browser-call
//
// The call flow:
//  1. Exotel dials the lead's phone (1 leg = 1x cost vs. 2x for bridge/human call).
//  2. While the phone rings, the agent opens /ws/agent?call_sid=XXX from the browser.
//  3. When the lead answers, Exotel connects to /media-stream. The wshandler
//     detects IsBridge=true and skips the AI pipeline, relaying audio to the
//     agent browser instead.
func (s *Server) browserCall(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	leadID, err := parseID(r, "lead_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead id")
		return
	}

	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}

	// Pull voice/language settings from the campaign so the Redis entry is
	// complete (wshandler reads it on start event), even though the AI pipeline
	// won't run in bridge mode.
	var vs any
	if campaignID > 0 {
		vs, _ = s.db.GetCampaignVoiceSettings(campaignID)
	}
	provider, voiceID, lang := extractVoice(vs)

	if s.initiator == nil {
		writeError(w, http.StatusServiceUnavailable, "dial service unavailable")
		return
	}

	ac := getAuth(r)
	leadName := strings.TrimSpace(lead.FirstName + " " + lead.LastName)

	data := dial.CallData{
		LeadID:      lead.ID,
		LeadName:    leadName,
		LeadPhone:   lead.Phone,
		CampaignID:  campaignID,
		OrgID:       ac.OrgID,
		Interest:    lead.Interest,
		TTSProvider: provider,
		TTSVoiceID:  voiceID,
		TTSLanguage: lang,
		IsBridge:    true,
		UserEmail:   ac.Email,
	}

	callSid, err := s.initiator.Initiate(r.Context(), data)
	if err != nil {
		s.logger.Warn("browserCall: initiate failed",
			zap.Int64("lead_id", leadID),
			zap.Int64("campaign_id", campaignID),
			zap.Error(err))
		writeError(w, dialErrorStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"call_sid":  callSid,
		"agent_url": fmt.Sprintf("/ws/agent?call_sid=%s", callSid),
		"status":    "dialing",
	})
}
