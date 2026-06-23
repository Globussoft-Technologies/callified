package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// executiveCreateRequest and executiveUpdateRequest hold the editable fields
// for an org-level executive.
type executiveCreateRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type executiveUpdateRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// listExecutives returns all executives for the authenticated org.
func (s *Server) listExecutives(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	list, err := s.db.GetExecutivesByOrg(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listExecutives", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(list))
}

// createExecutive creates a new executive under the authenticated org.
func (s *Server) createExecutive(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var req executiveCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeFieldError(w, http.StatusBadRequest, "name is required", map[string]string{"name": "Name is required"})
		return
	}
	id, err := s.db.CreateExecutive(ac.OrgID, name, req.Email, req.Phone)
	if err != nil {
		s.logger.Sugar().Errorw("createExecutive", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// updateExecutive updates an existing executive scoped to the authenticated org.
func (s *Server) updateExecutive(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req executiveUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeFieldError(w, http.StatusBadRequest, "name is required", map[string]string{"name": "Name is required"})
		return
	}
	if err := s.db.UpdateExecutive(id, ac.OrgID, name, req.Email, req.Phone); err != nil {
		s.logger.Sugar().Errorw("updateExecutive", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// deleteExecutive removes an executive from the authenticated org and unassigns
// any leads that referenced it.
func (s *Server) deleteExecutive(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteExecutive(id, ac.OrgID); err != nil {
		s.logger.Sugar().Errorw("deleteExecutive", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Best-effort: unassign leads that still reference this executive. The
	// ON DELETE is not enforced by a FK, so clean up manually.
	if err := s.db.UnassignExecutiveFromLeads(id, ac.OrgID); err != nil {
		s.logger.Sugar().Warnw("deleteExecutive: unassign failed", "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// setCampaignExecutives replaces the executives assigned to a campaign.
func (s *Server) setCampaignExecutives(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	var body struct {
		ExecutiveIDs []int64 `json:"executive_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Verify the campaign belongs to the caller's org before mutating it.
	campaign, err := s.db.GetCampaignByID(campaignID)
	if err != nil {
		s.logger.Sugar().Errorw("setCampaignExecutives", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if campaign == nil || campaign.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if err := s.db.SetCampaignExecutives(campaignID, body.ExecutiveIDs); err != nil {
		s.logger.Sugar().Errorw("setCampaignExecutives", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// parseExecutiveIDs parses a comma-separated list of executive IDs from a query
// parameter. Empty or missing values return nil.
func parseExecutiveIDs(q string) []int64 {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	parts := strings.Split(q, ",")
	var ids []int64
	seen := make(map[int64]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
