package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ── GET /api/exotel-accounts ─────────────────────────────────────────────────

func (s *Server) listExotelAccounts(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	accounts, err := s.db.GetOrgExotelAccounts(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listExotelAccounts", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(accounts))
}

// ── POST /api/exotel-accounts ────────────────────────────────────────────────

func (s *Server) createExotelAccount(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var req struct {
		Name       string `json:"name"`
		APIKey     string `json:"api_key"`
		APIToken   string `json:"api_token"`
		AccountSID string `json:"account_sid"`
		CallerID   string `json:"caller_id"`
		AppID      string `json:"app_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || req.APIKey == "" || req.APIToken == "" ||
		req.AccountSID == "" || req.CallerID == "" {
		writeError(w, http.StatusBadRequest, "name, api_key, api_token, account_sid and caller_id are required")
		return
	}
	id, err := s.db.CreateOrgExotelAccount(ac.OrgID, strings.TrimSpace(req.Name),
		req.APIKey, req.APIToken, req.AccountSID, req.CallerID, req.AppID)
	if err != nil {
		s.logger.Sugar().Errorw("createExotelAccount", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ── PUT /api/exotel-accounts/{id} ────────────────────────────────────────────

func (s *Server) updateExotelAccount(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name       string `json:"name"`
		APIKey     string `json:"api_key"`
		APIToken   string `json:"api_token"`
		AccountSID string `json:"account_sid"`
		CallerID   string `json:"caller_id"`
		AppID      string `json:"app_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || req.APIKey == "" || req.APIToken == "" ||
		req.AccountSID == "" || req.CallerID == "" {
		writeError(w, http.StatusBadRequest, "name, api_key, api_token, account_sid and caller_id are required")
		return
	}
	if err := s.db.UpdateOrgExotelAccount(id, ac.OrgID, strings.TrimSpace(req.Name),
		req.APIKey, req.APIToken, req.AccountSID, req.CallerID, req.AppID); err != nil {
		s.logger.Sugar().Errorw("updateExotelAccount", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}

// ── DELETE /api/exotel-accounts/{id} ─────────────────────────────────────────

func (s *Server) deleteExotelAccount(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteOrgExotelAccount(id, ac.OrgID); err != nil {
		s.logger.Sugar().Errorw("deleteExotelAccount", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ── GET /api/campaigns/{id}/exotel-account ───────────────────────────────────
// Returns which org-level account (if any) is linked to this campaign.

func (s *Server) getCampaignExotelAccount(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	accountID, _ := s.db.GetCampaignExotelAccountID(campaignID)
	writeJSON(w, http.StatusOK, map[string]int64{"exotel_account_id": accountID})
}

// ── PUT /api/campaigns/{id}/exotel-account ───────────────────────────────────
// Links or unlinks an org-level Exotel account on a campaign.

func (s *Server) setCampaignExotelAccount(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	var req struct {
		ExotelAccountID int64 `json:"exotel_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.db.SetCampaignExotelAccount(campaignID, req.ExotelAccountID); err != nil {
		s.logger.Sugar().Errorw("setCampaignExotelAccount", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}
