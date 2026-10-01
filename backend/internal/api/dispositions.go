package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

func (s *Server) campaignForDispositionRequest(w http.ResponseWriter, r *http.Request) (*db.Campaign, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return nil, false
	}
	campaign, err := s.db.GetCampaignByID(id)
	ac := getAuth(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if campaign == nil || campaign.OrgID != ac.OrgID || !s.canViewCampaign(ac, campaign.ID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return nil, false
	}
	return campaign, true
}

func (s *Server) getCampaignDispositionOptions(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.campaignForDispositionRequest(w, r)
	if !ok {
		return
	}
	options, err := s.db.GetCampaignDispositionOptions(campaign.OrgID, campaign.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load disposition options")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(options))
}

func (s *Server) putCampaignDispositionOptions(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.campaignForDispositionRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Options []db.DispositionOption `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.db.ReplaceCampaignDispositionOptions(campaign.OrgID, campaign.ID, body.Options); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	options, _ := s.db.GetCampaignDispositionOptions(campaign.OrgID, campaign.ID)
	writeJSON(w, http.StatusOK, emptyJSON(options))
}

func (s *Server) dispositionTranscript(w http.ResponseWriter, r *http.Request) (*db.Transcript, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transcript id")
		return nil, false
	}
	t, err := s.db.GetTranscriptByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	ac := getAuth(r)
	if t == nil || t.OrgID != ac.OrgID || (t.LeadID > 0 && !s.canAccessLead(ac, t.LeadID)) {
		writeError(w, http.StatusNotFound, "transcript not found")
		return nil, false
	}
	return t, true
}

func (s *Server) getTranscriptDisposition(w http.ResponseWriter, r *http.Request) {
	t, ok := s.dispositionTranscript(w, r)
	if !ok {
		return
	}
	disposition, err := s.db.GetCallDispositionByTranscript(t.OrgID, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load disposition")
		return
	}
	options, _ := s.db.GetCampaignDispositionOptions(t.OrgID, t.CampaignID)
	writeJSON(w, http.StatusOK, map[string]any{"disposition": disposition, "options": emptyJSON(options)})
}

func (s *Server) patchTranscriptDisposition(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "crm.edit") {
		return
	}
	t, ok := s.dispositionTranscript(w, r)
	if !ok {
		return
	}
	var body struct {
		Code    string `json:"code"`
		Summary string `json:"summary"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Code) == "" {
		writeError(w, http.StatusBadRequest, "disposition code is required")
		return
	}
	value, err := s.db.OverrideCallDisposition(t.OrgID, t.ID, getAuth(r).UserID, body.Code, body.Summary, body.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if value == nil {
		writeError(w, http.StatusConflict, "generate the AI disposition before overriding it")
		return
	}
	writeJSON(w, http.StatusOK, value)
}
