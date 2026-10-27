package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/dial"
)

// BrowserCallRequest is the optional body for POST /api/campaigns/{id}/leads/{lead_id}/browser-call.
type BrowserCallRequest struct {
	ExotelAccountID int64 `json:"exotel_account_id"`
	ScheduledCallID int64 `json:"scheduled_call_id"`
}

// BrowserCallResponse is returned by POST /api/campaigns/{id}/leads/{lead_id}/browser-call.
type BrowserCallResponse struct {
	CallSid  string `json:"call_sid"`
	AgentURL string `json:"agent_url"`
	Status   string `json:"status"`
}

// browserCall initiates a browser-to-phone call for a specific campaign lead.
//
// @Summary     Browser call for campaign lead
// @Description Initiates a browser-to-phone (agent bridge) call for a specific lead. The agent opens the returned agent_url via WebSocket to receive audio.
// @Tags        dialing
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id        path  int64                true  "Campaign ID"
// @Param       lead_id   path  int64                true  "Lead ID"
// @Param       body      body  BrowserCallRequest   false "Optional Exotel account or scheduled callback ID"
// @Success     200  {object}  BrowserCallResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     409  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/leads/{lead_id}/browser-call [post]
//
// The call flow:
//  1. Exotel dials the lead's phone (1 leg = 1x cost vs. 2x for bridge/human call).
//  2. While the phone rings, the agent opens /ws/agent?call_sid=XXX from the browser.
//  3. When the lead answers, Exotel connects to /media-stream. The wshandler
//     detects IsBridge=true and skips the AI pipeline, relaying audio to the
//     agent browser instead.
func (s *Server) browserCall(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "calls.browser_call") {
		return
	}
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	campaignID := campaign.ID
	leadID, err := parseID(r, "lead_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead id")
		return
	}

	ac := getAuth(r)

	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}
	if !s.canAccessCampaignLead(ac, campaignID, lead.ID) {
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
	provider, voiceID, lang, maxCallDurationSeconds := extractVoice(vs)

	if s.initiator == nil {
		writeError(w, http.StatusServiceUnavailable, "dial service unavailable")
		return
	}

	leadName := strings.TrimSpace(lead.FirstName + " " + lead.LastName)

	var body BrowserCallRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	var scheduledByUserID int64
	if body.ScheduledCallID > 0 {
		u, err := s.db.GetUserByEmail(ac.Email)
		if err != nil || u == nil {
			writeError(w, http.StatusUnauthorized, "invalid user")
			return
		}
		scheduledByUserID = u.ID
		claimed, err := s.db.ClaimScheduledCallForManualDial(ac.OrgID, body.ScheduledCallID, scheduledByUserID, lead.ID, campaignID)
		if err != nil {
			s.logger.Warn("browserCall: claim scheduled call failed",
				zap.Int64("scheduled_call_id", body.ScheduledCallID),
				zap.Int64("lead_id", leadID),
				zap.Int64("campaign_id", campaignID),
				zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !claimed {
			writeError(w, http.StatusConflict, "scheduled callback is no longer pending")
			return
		}
	}

	data := dial.CallData{
		LeadID:                 lead.ID,
		LeadName:               leadName,
		LeadPhone:              lead.Phone,
		CampaignID:             campaignID,
		OrgID:                  ac.OrgID,
		Interest:               lead.Interest,
		TTSProvider:            provider,
		TTSVoiceID:             voiceID,
		TTSLanguage:            lang,
		MaxCallDurationSeconds: maxCallDurationSeconds,
		IsBridge:               true,
		UserEmail:              ac.Email,
		UserID:                 userIDForDial(ac),
		ExotelAccountID:        body.ExotelAccountID,
	}

	callSid, err := s.initiator.Initiate(r.Context(), data)
	if err != nil {
		if body.ScheduledCallID > 0 && scheduledByUserID > 0 {
			if resetErr := s.db.ResetScheduledCallToPending(ac.OrgID, body.ScheduledCallID, scheduledByUserID); resetErr != nil {
				s.logger.Warn("browserCall: reset scheduled call failed",
					zap.Int64("scheduled_call_id", body.ScheduledCallID),
					zap.Error(resetErr))
			}
		}
		s.logger.Warn("browserCall: initiate failed",
			zap.Int64("lead_id", leadID),
			zap.Int64("campaign_id", campaignID),
			zap.Error(err))
		writeError(w, dialErrorStatus(err), err.Error())
		return
	}

	ticket, err := dial.AgentTicket(s.cfg.JWTSecret, callSid, ac.OrgID, userIDForDial(ac))
	if err != nil {
		// Do not leave a paid carrier call ringing when the browser cannot join.
		if hangupErr := s.initiator.Hangup(r.Context(), callSid, campaignID); hangupErr != nil {
			s.logger.Warn("browserCall: authorization cleanup failed", zap.Error(hangupErr))
		}
		writeError(w, http.StatusInternalServerError, "could not authorize browser call")
		return
	}
	writeJSON(w, http.StatusOK, BrowserCallResponse{
		CallSid:  callSid,
		AgentURL: fmt.Sprintf("/ws/agent?call_sid=%s&ticket=%s", url.QueryEscape(callSid), url.QueryEscape(ticket)),
		Status:   "dialing",
	})
}

// agentTicket authorizes the authenticated call owner before the browser opens
// its audio WebSocket. A call identifier alone is not permission to join it.
func (s *Server) agentTicket(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "calls.browser_call") {
		return
	}
	ac := getAuth(r)
	callSID := r.PathValue("call_sid")
	pending, ok := s.store.GetPendingCall(r.Context(), callSID)
	if !ok || !pending.IsBridge || pending.OrgID != ac.OrgID || pending.UserID != userIDForDial(ac) || pending.UserEmail != ac.Email {
		writeError(w, http.StatusNotFound, "browser call not found")
		return
	}
	ticket, err := dial.AgentTicket(s.cfg.JWTSecret, callSID, pending.OrgID, pending.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create browser call ticket")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket, "expires_in": 120})
}

// hangupBrowserCall also works during ringing, before a carrier media session
// exists. A WebSocket-only hangup cannot cancel a not-yet-answered Plivo call.
func (s *Server) hangupBrowserCall(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "calls.browser_call") {
		return
	}
	ac := getAuth(r)
	callSID := r.PathValue("call_sid")
	pending, ok := s.store.GetPendingCall(r.Context(), callSID)
	if !ok || !pending.IsBridge || pending.OrgID != ac.OrgID || pending.UserID != userIDForDial(ac) || pending.UserEmail != ac.Email {
		writeError(w, http.StatusNotFound, "browser call not found")
		return
	}
	if s.initiator == nil {
		writeError(w, http.StatusServiceUnavailable, "dial service unavailable")
		return
	}
	if err := s.initiator.Hangup(r.Context(), callSID, pending.CampaignID); err != nil {
		s.logger.Warn("browser call hangup failed", zap.String("call_sid", callSID), zap.Error(err))
		writeError(w, http.StatusBadGateway, "could not hang up browser call")
		return
	}
	if s.wsHandler != nil {
		s.wsHandler.CloseCall(callSID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}
