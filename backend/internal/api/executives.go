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
	if !s.requirePermission(w, r, "executives.manage") {
		return
	}
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
	if !s.requirePermission(w, r, "executives.manage") {
		return
	}
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
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		writeFieldError(w, http.StatusBadRequest, "email is required", map[string]string{"email": "Email is required"})
		return
	}
	linkedUser, err := s.db.GetUserByEmail(email)
	if err != nil {
		s.logger.Sugar().Errorw("createExecutive: linked user lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if linkedUser == nil || linkedUser.OrgID != ac.OrgID || linkedUser.Role != "Executive" {
		writeFieldError(w, http.StatusBadRequest, "Create this email as an Executive team member first.", map[string]string{"email": "Email must belong to an Executive team member"})
		return
	}
	if existing, err := s.db.GetExecutiveByEmail(ac.OrgID, email); err != nil {
		s.logger.Sugar().Errorw("createExecutive: existing lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if existing != nil {
		if err := s.db.UpdateExecutive(existing.ID, ac.OrgID, name, email, req.Phone); err != nil {
			s.logger.Sugar().Errorw("createExecutive: update existing", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"id": existing.ID})
		return
	}
	id, err := s.db.CreateExecutive(ac.OrgID, name, email, req.Phone)
	if err != nil {
		s.logger.Sugar().Errorw("createExecutive", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// updateExecutive updates an existing executive scoped to the authenticated org.
func (s *Server) updateExecutive(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "executives.manage") {
		return
	}
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
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		writeFieldError(w, http.StatusBadRequest, "email is required", map[string]string{"email": "Email is required"})
		return
	}
	linkedUser, err := s.db.GetUserByEmail(email)
	if err != nil {
		s.logger.Sugar().Errorw("updateExecutive: linked user lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if linkedUser == nil || linkedUser.OrgID != ac.OrgID || linkedUser.Role != "Executive" {
		writeFieldError(w, http.StatusBadRequest, "Create this email as an Executive team member first.", map[string]string{"email": "Email must belong to an Executive team member"})
		return
	}
	if err := s.db.UpdateExecutive(id, ac.OrgID, name, email, req.Phone); err != nil {
		s.logger.Sugar().Errorw("updateExecutive", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// deleteExecutive removes an executive from the authenticated org and unassigns
// any leads that referenced it.
func (s *Server) deleteExecutive(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "executives.manage") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	exec, err := s.db.GetExecutiveByID(id, ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("deleteExecutive: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if exec == nil {
		writeError(w, http.StatusNotFound, "executive not found")
		return
	}
	if exec.Email != "" {
		caller, err := s.db.GetUserByEmail(ac.Email)
		if err != nil || caller == nil {
			writeError(w, http.StatusInternalServerError, "could not resolve caller")
			return
		}
		linkedUser, err := s.db.GetUserByEmail(strings.TrimSpace(exec.Email))
		if err != nil {
			s.logger.Sugar().Errorw("deleteExecutive: linked user lookup", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if linkedUser != nil && linkedUser.OrgID == ac.OrgID {
			if linkedUser.ID == caller.ID {
				writeError(w, http.StatusForbidden, "you cannot remove your own account")
				return
			}
			if linkedUser.Role == "Admin" {
				count, err := s.db.CountAdminsInOrg(ac.OrgID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "internal error")
					return
				}
				if count <= 1 {
					writeError(w, http.StatusForbidden, "cannot remove the last remaining admin")
					return
				}
			}
			if err := s.db.DeleteUser(linkedUser.ID, ac.OrgID); err != nil {
				s.logger.Sugar().Errorw("deleteExecutive: delete linked user", "err", err)
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
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
	if !s.requirePermission(w, r, "campaigns.assign_users") {
		return
	}
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
	// Reject any executive IDs that do not belong to the caller's org.
	if len(body.ExecutiveIDs) > 0 {
		matchCount, err := s.db.CountExecutivesByOrgAndIDs(ac.OrgID, body.ExecutiveIDs)
		if err != nil {
			s.logger.Sugar().Errorw("setCampaignExecutives", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if int(matchCount) != len(body.ExecutiveIDs) {
			writeError(w, http.StatusBadRequest, "one or more executives do not belong to this org")
			return
		}
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
