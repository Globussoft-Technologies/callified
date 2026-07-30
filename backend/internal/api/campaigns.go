package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
)

// maxCampaignLeads is the hard cap on the number of leads a single campaign can hold.
const maxCampaignLeads = 100_000

// ImportRejection describes one CSV row that could not be imported.
type ImportRejection struct {
	Row       int    `json:"row"`
	FirstName string `json:"first_name"`
	Phone     string `json:"phone"`
	Reason    string `json:"reason"`
}

// ── GET /api/campaigns ───────────────────────────────────────────────────────

// @Summary     List campaigns
// @Description Returns all campaigns for the org. Requires Admin or Agent role.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.Campaign
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns [get]
func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var campaigns []db.Campaign
	var err error
	if s.isSuperAdmin(ac.Email) && ac.OrgID <= 0 {
		campaigns, err = s.db.GetAllCampaigns()
	} else {
		campaigns, err = s.listCampaignsForUser(ac)
	}
	if err != nil {
		s.logger.Sugar().Errorw("listCampaigns", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(campaigns))
}

// listCampaignsForUser returns campaigns visible to the authenticated user
// based on their resolved dashboard role.
func (s *Server) listCampaignsForUser(ac AuthClaims) ([]db.Campaign, error) {
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return []db.Campaign{}, nil
	}

	role := user.Role
	if s.isSuperAdmin(ac.Email) {
		role = db.RoleAdmin
	}

	switch role {
	case db.RoleAdmin:
		return s.db.GetCampaignsByOrg(ac.OrgID)
	case db.RoleTeamLeader:
		ids, err := s.db.GetCampaignsForManager(user.ID)
		if err != nil {
			return nil, err
		}
		return s.db.GetCampaignsByIDs(ids)
	case db.RoleAgent:
		ids, err := s.db.GetCampaignsForUser(user.ID)
		if err != nil {
			return nil, err
		}
		return s.db.GetCampaignsByIDs(ids)
	default:
		return []db.Campaign{}, nil
	}
}

// ── POST /api/campaigns ──────────────────────────────────────────────────────

type campaignCreateRequest struct {
	Name            string `json:"name"`
	ProductID       int64  `json:"product_id"`
	LeadSource      string `json:"lead_source"`
	Channel         string `json:"channel"`
	ExotelAccountID int64  `json:"exotel_account_id"`
}

// validateCampaignName mirrors frontend/src/utils/campaignName.js. Defense
// in depth — the React UI auto-escapes JSX text, but the same string can
// leak into less-defended surfaces (emails, CSV exports, plain-text logs)
// where `<` / `>` would matter, so we reject them at the API boundary too.
func validateCampaignName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "name is required"
	}
	if len(trimmed) > 100 {
		return "name must be 100 characters or fewer"
	}
	if strings.ContainsAny(trimmed, "<>") {
		return "name cannot contain < or > characters"
	}
	return ""
}

// @Summary     Create campaign
// @Description Creates a new calling campaign. Requires Admin role.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      campaignCreateRequest  true  "Campaign data"
// @Success     201   {object}  IDResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns [post]
func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var req campaignCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProductID == 0 {
		writeError(w, http.StatusBadRequest, "name and product_id required")
		return
	}
	if msg := validateCampaignName(req.Name); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	exotelAccountID := req.ExotelAccountID
	if exotelAccountID == 0 {
		if accounts, acctErr := s.db.GetOrgExotelAccounts(ac.OrgID); acctErr == nil && len(accounts) > 0 {
			exotelAccountID = accounts[0].ID
		}
	}
	id, err := s.db.CreateCampaign(ac.OrgID, req.ProductID, strings.TrimSpace(req.Name), req.LeadSource, coalesceStr(req.Channel, "voice"), exotelAccountID)
	if err != nil {
		s.logger.Sugar().Errorw("createCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ── GET /api/campaigns/{id} ──────────────────────────────────────────────────
//
// Attaches up-to-date stats (and voice settings) on the same response, so the
// Campaign Detail page renders live numbers on every open — not whatever stale
// snapshot the campaigns-list fetch had. Matches the Python endpoint shape
// (routes.py:1335-1341) exactly:
//
//     {...campaign_fields, "stats": {...}, "voice_settings": {...}}
//
// Before this change, the Total/Called/Qualified/Appointments KPI cards read
// from selectedCampaign.stats, which the list endpoint populates once — so any
// call or lead add that happened after the list was fetched left the cards
// frozen at 0 until a full page reload.

// @Summary     Get campaign
// @Description Returns a campaign with fresh stats and voice settings. Requires Admin or Agent role.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  db.Campaign
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id} [get]
func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := s.db.GetCampaignByID(id)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if c == nil || c.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if !s.canViewCampaign(ac, c.ID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	// Attach fresh stats — best-effort; we don't fail the whole response if
	// the stats query breaks. Stats are filtered by lead access for non-Admins.
	execIDs, apply, _ := s.leadAccessExecIDs(ac)
	if stats, err := s.db.GetCampaignStats(id, execIDs, apply); err == nil {
		c.Stats = &stats
	} else {
		s.logger.Sugar().Warnw("getCampaign: stats lookup failed", "id", id, "err", err)
	}
	// Attach voice settings in a merged map so we stay backwards-compatible
	// with clients that still read c.* directly.
	resp := map[string]any{
		"id":           c.ID,
		"org_id":       c.OrgID,
		"product_id":   c.ProductID,
		"name":         c.Name,
		"status":       c.Status,
		"tts_provider": c.TTSProvider,
		"tts_voice_id": c.TTSVoiceID,
		"tts_language": c.TTSLanguage,
		"lead_source":  c.LeadSource,
		"channel":      c.Channel,
		"product_name": c.ProductName,
		"created_at":   c.CreatedAt,
		"stats":        c.Stats,
	}
	if vs, err := s.db.GetCampaignVoiceSettings(id); err == nil {
		resp["voice_settings"] = map[string]string{
			"tts_provider": vs.TTSProvider,
			"tts_voice_id": vs.TTSVoiceID,
			"tts_language": vs.TTSLanguage,
		}
	}
	if execIDs, err := s.db.GetCampaignExecutiveIDs(id); err == nil {
		resp["executive_ids"] = execIDs
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── PUT /api/campaigns/{id} ──────────────────────────────────────────────────

type campaignUpdateRequest struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	LeadSource string `json:"lead_source"`
	ProductID  int64  `json:"product_id"`
	Channel    string `json:"channel"`
}

// @Summary     Update campaign
// @Description Updates campaign name, status, or product. Requires Admin role.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      int64                  true  "Campaign ID"
// @Param       body  body      campaignUpdateRequest  true  "Updated fields (empty fields are ignored)"
// @Success     200   {object}  BoolResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id} [put]
func (s *Server) updateCampaign(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	campaign, err := s.db.GetCampaignByID(id)
	if err != nil {
		s.logger.Sugar().Errorw("updateCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if campaign == nil || campaign.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	var req campaignUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Only validate name when the caller is actually changing it. Empty Name
	// in this PATCH-style endpoint means "leave as-is" (UpdateCampaign
	// already skips empty fields).
	if req.Name != "" {
		if msg := validateCampaignName(req.Name); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
	}
	if err := s.db.UpdateCampaign(id, req.Name, req.Status, req.LeadSource, req.Channel, req.ProductID); err != nil {
		s.logger.Sugar().Errorw("updateCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── DELETE /api/campaigns/{id} ───────────────────────────────────────────────

// @Summary     Delete campaign
// @Description Permanently deletes a campaign. Requires Admin role.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id} [delete]
func (s *Server) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	campaign, err := s.db.GetCampaignByID(id)
	if err != nil {
		s.logger.Sugar().Errorw("deleteCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if campaign == nil || campaign.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	deleted, err := s.db.DeleteCampaign(id)
	if err != nil {
		s.logger.Sugar().Errorw("deleteCampaign", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ── GET /api/campaigns/{id}/leads ────────────────────────────────────────────

// PaginatedCampaignLeadsResponse is the shape returned by listCampaignLeads.
type PaginatedCampaignLeadsResponse struct {
	Leads []db.CampaignLead `json:"leads"`
	Total int64             `json:"total"`
	Page  int64             `json:"page"`
	Limit int64             `json:"limit"`
}

// @Summary     List campaign leads
// @Description Returns a paginated list of leads enrolled in a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id              path   int64  true   "Campaign ID"
// @Param       page            query  int    false  "Page number (1-based)"  default(1)
// @Param       limit           query  int    false  "Page size"              default(100)
// @Param       search          query  string false  "Search by name/phone/source"
// @Param       executive_ids   query  string false  "Comma-separated executive IDs"
// @Param       scheduled_from  query  string false  "Scheduled from (ISO datetime)"
// @Param       scheduled_to    query  string false  "Scheduled to (ISO datetime)"
// @Success     200  {object}  PaginatedCampaignLeadsResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/leads [get]
func (s *Server) listCampaignLeads(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	ac := getAuth(r)
	q := r.URL.Query()
	filter := db.CampaignLeadsFilter{
		CampaignID:    campaign.ID,
		ExecIDs:       parseExecutiveIDs(q.Get("executive_ids")),
		Search:        q.Get("search"),
		ScheduledFrom: q.Get("scheduled_from"),
		ScheduledTo:   q.Get("scheduled_to"),
	}
	// Enforce per-lead isolation for Agents and Team Leaders.
	allowed, apply, err := s.leadAccessExecIDs(ac)
	if err != nil {
		s.logger.Sugar().Errorw("listCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if apply {
		filter.ExecIDs = allowed
	}
	page, limit := parsePagination(r, 100, 500)
	offset := (page - 1) * limit

	leads, err := s.db.GetCampaignLeadsPaginated(filter, limit, offset)
	if err != nil {
		s.logger.Sugar().Errorw("listCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	total, err := s.db.CountCampaignLeads(filter)
	if err != nil {
		s.logger.Sugar().Errorw("listCampaignLeads count", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, PaginatedCampaignLeadsResponse{
		Leads: emptyJSON(leads),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// ── POST /api/campaigns/{id}/leads ───────────────────────────────────────────

// @Summary     Add leads to campaign
// @Description Enrols existing leads into a campaign. Requires Admin role.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      int64                          true  "Campaign ID"
// @Param       body  body      object{lead_ids=[]int64}       true  "Lead IDs to enrol"
// @Success     200   {object}  object{added=int}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id}/leads [post]
func (s *Server) addCampaignLeads(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	var body struct {
		LeadIDs []int64 `json:"lead_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.LeadIDs) == 0 {
		writeError(w, http.StatusBadRequest, "lead_ids required")
		return
	}
	// Reject any lead IDs that do not belong to the caller's org.
	matchCount, err := s.db.CountLeadsByOrgAndIDs(ac.OrgID, body.LeadIDs)
	if err != nil {
		s.logger.Sugar().Errorw("addCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if int(matchCount) != len(body.LeadIDs) {
		writeError(w, http.StatusBadRequest, "one or more leads do not belong to this org")
		return
	}
	added, err := s.db.AddLeadsToCampaign(campaign.ID, body.LeadIDs)
	if err != nil {
		s.logger.Sugar().Errorw("addCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}

// ── DELETE /api/campaigns/{id}/leads/{lead_id} ───────────────────────────────

// @Summary     Remove lead from campaign
// @Description Removes a lead from a campaign. Requires Admin role.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id       path  int64  true  "Campaign ID"
// @Param       lead_id  path  int64  true  "Lead ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/leads/{lead_id} [delete]
func (s *Server) removeCampaignLead(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	campaignID := campaign.ID
	leadID, err := parseID(r, "lead_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead_id")
		return
	}
	removed, err := s.db.RemoveLeadFromCampaign(campaignID, leadID)
	if err != nil {
		s.logger.Sugar().Errorw("removeCampaignLead", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "lead not in campaign")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// ── GET /api/campaigns/{id}/stats ────────────────────────────────────────────

// @Summary     Get campaign stats
// @Description Returns KPI counts (total, called, qualified, appointments) for a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  db.CampaignStats
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/stats [get]
func (s *Server) getCampaignStats(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	execIDs, apply, _ := s.leadAccessExecIDs(ac)
	stats, err := s.db.GetCampaignStats(campaign.ID, execIDs, apply)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignStats", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ── GET /api/campaigns/{id}/call-outcome-stats ───────────────────────────────

// @Summary     Get campaign call outcome stats
// @Description Returns the number of total, connected, completed, unanswered, busy, and failed calls for a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  db.CallOutcomeStats
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/call-outcome-stats [get]
func (s *Server) getCampaignCallOutcomeStats(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	execIDs, apply, _ := s.leadAccessExecIDs(ac)
	stats, err := s.db.GetCampaignCallOutcomeStats(campaign.ID, execIDs, apply)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignCallOutcomeStats", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ── GET /api/campaigns/{id}/call-log ─────────────────────────────────────────

// @Summary     Get campaign call log
// @Description Returns the full call history for all leads in a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {array}   db.CallLogEntry
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/call-log [get]
func (s *Server) getCampaignCallLog(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	ac := getAuth(r)
	execIDs, apply, err := s.leadAccessExecIDs(ac)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignCallLog", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !apply {
		execIDs = nil
	}
	log, err := s.db.GetCampaignCallLog(campaign.ID, execIDs)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignCallLog", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(log))
}

// ── GET /api/campaigns/{id}/export-recordings ────────────────────────────────
// Downloads a CSV of all calls in the campaign that have a recording URL.
func (s *Server) exportRecordings(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	ac := getAuth(r)
	execIDs, apply, err := s.leadAccessExecIDs(ac)
	if err != nil {
		s.logger.Sugar().Errorw("exportRecordings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !apply {
		execIDs = nil
	}
	entries, err := s.db.GetCampaignRecordingsExport(campaign.ID, execIDs)
	if err != nil {
		s.logger.Sugar().Errorw("exportRecordings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	fname := fmt.Sprintf("recordings_%s.csv", strings.ReplaceAll(campaign.Name, " ", "_"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	wr := csv.NewWriter(w)
	_ = wr.Write([]string{
		"Name", "Phone", "Campaign", "Lead Status", "Call Type", "Call Date/Time",
		"Duration (s)", "Outcome", "Follow-up Note", "Recording Filename", "Recording URL",
	})
	for _, e := range entries {
		_ = wr.Write([]string{
			e.Name, e.Phone, campaign.Name, e.LeadStatus, e.CallType, e.CreatedAt,
			fmt.Sprintf("%.0f", e.Duration), e.Outcome, e.FollowUpNote,
			e.RecordingFilename, e.RecordingURL,
		})
	}
	wr.Flush()
}

// exportCampaignLeads streams all leads in a campaign as a CSV download.
// Unlike exportRecordings, this exports the lead list (not call recordings)
// and is designed to handle 100k+ rows without loading them into memory.
func (s *Server) exportCampaignLeads(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	ac := getAuth(r)
	execIDs, apply, err := s.leadAccessExecIDs(ac)
	if err != nil {
		s.logger.Sugar().Errorw("exportCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !apply {
		execIDs = nil
	}
	fname := fmt.Sprintf("leads_%s.csv", strings.ReplaceAll(campaign.Name, " ", "_"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	if err := s.db.ExportCampaignLeads(campaign.ID, execIDs, w); err != nil {
		s.logger.Sugar().Errorw("exportCampaignLeads", "campaign_id", campaign.ID, "err", err)
	}
}

// ── GET /api/campaigns/{id}/voice-settings ───────────────────────────────────

// @Summary     Get campaign voice settings
// @Description Returns TTS provider, voice ID and language for a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  db.VoiceSettings
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/voice-settings [get]
func (s *Server) getCampaignVoiceSettings(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	vs, err := s.db.GetCampaignVoiceSettings(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignVoiceSettings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// ── PUT /api/campaigns/{id}/voice-settings ────────────────────────────────────

// @Summary     Save campaign voice settings
// @Description Updates TTS provider, voice ID and language for a campaign. Also invalidates Redis voice cache. Requires Admin role.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64             true  "Campaign ID"
// @Param       body  body  db.VoiceSettings  true  "Voice settings"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/voice-settings [put]
func (s *Server) saveCampaignVoiceSettings(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	id := campaign.ID
	var vs db.VoiceSettings
	if err := json.NewDecoder(r.Body).Decode(&vs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.db.SaveCampaignVoiceSettings(id, vs); err != nil {
		s.logger.Sugar().Errorw("saveCampaignVoiceSettings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Invalidate the per-lead voice cache for every lead in this campaign.
	// Without this, a freshly-saved campaign voice would be silently
	// overridden by the lead_voice:{id} Redis key (90-day TTL) the next
	// time a real Dial is made to a lead this campaign has called before.
	// Best-effort: log on error but still report Save success — the cache
	// has a 90-day expiry, so worst case the user retries later.
	if s.store != nil {
		ids, listErr := s.db.ListCampaignLeadIDs(id)
		if listErr != nil {
			s.logger.Sugar().Warnw("saveCampaignVoiceSettings: ListCampaignLeadIDs failed; cache not invalidated",
				"campaign_id", id, "err", listErr)
		} else {
			for _, leadID := range ids {
				s.store.DeleteRaw(r.Context(), fmt.Sprintf("lead_voice:%d", leadID))
			}
			s.logger.Sugar().Infow("saveCampaignVoiceSettings: invalidated lead_voice cache",
				"campaign_id", id, "lead_count", len(ids))
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}

// ── POST /api/campaigns/{id}/import-csv ──────────────────────────────────────
// Import CSV of leads and add them to the campaign in one step.

// @Summary     Import campaign leads from CSV
// @Description Bulk-imports leads from a CSV and immediately enrols them in the campaign. Requires Admin role.
// @Tags        campaigns
// @Accept      multipart/form-data
// @Produce     json
// @Security    BearerAuth
// @Param       id    path      int64  true  "Campaign ID"
// @Param       file  formData  file   true  "CSV file (columns: first_name, last_name, phone, company, source)"
// @Success     200   {object}  object{imported=int,added_to_campaign=int,rejected=[]ImportRejection,errors=[]string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id}/import-csv [post]
func (s *Server) importCampaignLeadsCSV(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	campaignID := campaign.ID
	// Allow large CSVs up to ~100 MB; files bigger than memory are spilled to disk.
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil || len(records) < 2 {
		writeError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	header := records[0]
	idx := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}
	iFirst, iLast, iPhone, iCompany, iSource := idx("first_name"), idx("last_name"), idx("phone"), idx("company"), idx("source")
	if iFirst < 0 || iPhone < 0 {
		writeError(w, http.StatusBadRequest, "CSV must have first_name and phone columns")
		return
	}

	get := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	var rejected []ImportRejection
	var rows []db.LeadImportRow
	seen := make(map[string]int) // phone -> first CSV row number where it appeared
	for rowIdx, rec := range records[1:] {
		rowNum := rowIdx + 2 // CSV rows are 1-based; header is row 1
		phone := normalizePhone(get(rec, iPhone))
		firstName := get(rec, iFirst)

		if phone == "" {
			rejected = append(rejected, ImportRejection{
				Row: rowNum, FirstName: firstName, Phone: get(rec, iPhone), Reason: "invalid or empty phone",
			})
			continue
		}
		if prevRow, ok := seen[phone]; ok {
			rejected = append(rejected, ImportRejection{
				Row: rowNum, FirstName: firstName, Phone: phone,
				Reason: fmt.Sprintf("duplicate phone in CSV (first seen at row %d)", prevRow),
			})
			continue
		}
		seen[phone] = rowNum
		rows = append(rows, db.LeadImportRow{
			Row:       rowNum,
			FirstName: firstName,
			LastName:  get(rec, iLast),
			Phone:     phone,
			Company:   get(rec, iCompany),
			Source:    get(rec, iSource),
		})
	}

	// Find which phones already exist in this org and which leads are already in the campaign.
	phones := make([]string, len(rows))
	for i, r := range rows {
		phones[i] = r.Phone
	}
	existing, err := s.db.GetLeadIDsByPhones(ac.OrgID, phones)
	if err != nil {
		s.logger.Sugar().Errorw("importCampaignLeadsCSV: GetLeadIDsByPhones", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	campaignLeadIDs, err := s.db.GetCampaignLeadIDs(campaignID)
	if err != nil {
		s.logger.Sugar().Errorw("importCampaignLeadsCSV: GetCampaignLeadIDs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Split valid rows into genuinely new leads and existing leads that should be updated.
	var newRows []db.LeadImportRow
	var existingToAdd int
	var updatedNames int
	for _, r := range rows {
		if id, ok := existing[r.Phone]; ok {
			// Phone already exists: update the lead's name from the CSV (phone stays unique).
			if updErr := s.db.UpdateLeadName(id, r.FirstName, r.LastName); updErr != nil {
				s.logger.Sugar().Errorw("importCampaignLeadsCSV: UpdateLeadName failed", "lead_id", id, "err", updErr)
			} else {
				updatedNames++
			}
			if campaignLeadIDs[id] {
				// Already in campaign; name was refreshed above.
			} else {
				existingToAdd++
			}
		} else {
			newRows = append(newRows, r)
		}
	}

	currentCount := int64(len(campaignLeadIDs))
	if currentCount+int64(len(newRows)+existingToAdd) > maxCampaignLeads {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("campaign lead limit exceeded: maximum %d leads (current %d, this import would add %d new)",
				maxCampaignLeads, currentCount, len(newRows)+existingToAdd))
		return
	}

	// Create only the genuinely new leads. Existing leads are left untouched.
	imported, errs := s.db.BulkCreateLeads(newRows, ac.OrgID)

	// Convert DB-level errors into structured rejections.
	var remainingErrors []string
	for _, e := range errs {
		var rowNum int
		prefix := "Row "
		if strings.HasPrefix(e, prefix) {
			if _, scanErr := fmt.Sscanf(e, "Row %d: ", &rowNum); scanErr == nil {
				reason := strings.TrimPrefix(e, fmt.Sprintf("Row %d: ", rowNum))
				found := false
				for _, nr := range newRows {
					if nr.Row == rowNum {
						rejected = append(rejected, ImportRejection{
							Row: nr.Row, FirstName: nr.FirstName, Phone: nr.Phone, Reason: reason,
						})
						found = true
						break
					}
				}
				if !found {
					remainingErrors = append(remainingErrors, e)
				}
				continue
			}
		}
		remainingErrors = append(remainingErrors, e)
	}

	// Re-resolve lead IDs after insert and add every row (new + existing) to the campaign once.
	leadMap, err := s.db.GetLeadIDsByPhones(ac.OrgID, phones)
	if err != nil {
		s.logger.Sugar().Errorw("importCampaignLeadsCSV: GetLeadIDsByPhones post-create", "err", err)
	}
	var addIDs []int64
	for _, r := range rows {
		if id, ok := leadMap[r.Phone]; ok && !campaignLeadIDs[id] {
			addIDs = append(addIDs, id)
		}
	}
	var addedToCampaign int
	if len(addIDs) > 0 {
		addedToCampaign, _ = s.db.AddLeadsToCampaign(campaignID, addIDs)
	}

	const maxReturnedRejected = 500
	rejectedTotal := len(rejected)
	if rejectedTotal > maxReturnedRejected {
		rejected = rejected[:maxReturnedRejected]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported":          imported,
		"added_to_campaign": addedToCampaign,
		"updated":           updatedNames,
		"rejected":          rejected,
		"rejected_total":    rejectedTotal,
		"errors":            remainingErrors,
	})
}

// ── GET /api/campaigns/{id}/exotel-creds ─────────────────────────────────────

// @Summary     Get campaign Exotel credentials
// @Description Returns the Exotel credentials stored for a campaign. All fields empty means the platform default is used.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  db.ExotelCreds
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/exotel-creds [get]
func (s *Server) getCampaignExotelCreds(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	creds, err := s.db.GetCampaignExotelCreds(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignExotelCreds", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

// ── PUT /api/campaigns/{id}/exotel-creds ─────────────────────────────────────

// @Summary     Save campaign Exotel credentials
// @Description Stores per-campaign Exotel API credentials. Pass empty strings to revert to platform defaults.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64            true  "Campaign ID"
// @Param       body  body  db.ExotelCreds   true  "Exotel credentials"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/exotel-creds [put]
func (s *Server) saveCampaignExotelCreds(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	var creds db.ExotelCreds
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.db.SaveCampaignExotelCreds(campaign.ID, creds); err != nil {
		s.logger.Sugar().Errorw("saveCampaignExotelCreds", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}

// ── GET /api/campaigns/{id}/call-reviews ──────────────────────────────────────

// @Summary     Get campaign call reviews
// @Description Returns AI-generated call quality reviews for all calls in a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {array}   object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/call-reviews [get]
func (s *Server) getCampaignCallReviews(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	reviews, err := s.db.GetCallReviewsByCampaign(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignCallReviews", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(reviews))
}

// ── GET /api/campaigns/{id}/retries ───────────────────────────────────────────
// Returns retries enriched with the lead's first_name/last_name/phone so the
// Retries tab renders without a second fetch. The route was missing entirely
// before — the tab silently fell back to its empty state. Issue #77.

// @Summary     Get campaign retries
// @Description Returns pending/failed call retries enriched with lead details.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {array}   db.RetryWithLead
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/retries [get]
func (s *Server) getCampaignRetries(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	retries, err := s.db.GetRetriesByCampaignWithLead(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignRetries", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(retries))
}

// ── GET /api/campaigns/{id}/call-insights ─────────────────────────────────────
// Aggregates call_reviews rows for a campaign into the summary cards +
// improvement/failure lists the Insights tab renders. Was missing entirely
// before — the tab fell back to the empty per-call list and showed the
// "no reviews yet" empty state forever. Issue #75.

// @Summary     Get campaign call insights
// @Description Aggregates call reviews into summary cards, improvement areas, and failure reasons.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/call-insights [get]
func (s *Server) getCampaignCallInsights(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	insights, err := s.db.GetCampaignCallInsights(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignCallInsights", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

// ── POST /api/campaigns/{id}/human-call/{lead_id} ────────────────────────────
// Initiates a human (agent-to-customer) call via Exotel Architecture 3:
// Exotel calls the agent first; when the agent picks up Exotel fetches our
// ExoML webhook which announces the customer name and bridges in the customer.

// @Summary     Initiate human call
// @Description Dials the agent's phone first via Exotel, then bridges to the lead's phone. Uses campaign-level Exotel credentials.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id       path  int64   true  "Campaign ID"
// @Param       lead_id  path  int64   true  "Lead ID"
// @Param       body     body  object  true  "Agent phone"
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/human-call/{lead_id} [post]
func (s *Server) humanCallLead(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
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

	var req struct {
		AgentPhone string `json:"agent_phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AgentPhone) == "" {
		writeError(w, http.StatusBadRequest, "agent_phone required")
		return
	}

	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}
	if !s.canAccessLead(ac, lead.ID) {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}

	creds, err := s.db.GetCampaignExotelCreds(campaignID)
	if err != nil || !creds.IsSet() {
		writeError(w, http.StatusBadRequest, "no Exotel credentials configured for this campaign")
		return
	}

	exotelClient := dial.NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType, creds.Region, creds.Subdomain)

	// StatusCallback delivers recording URL + final status when the call ends.
	statusCallback := fmt.Sprintf("%s/webhook/exotel/status?lead_id=%d&campaign_id=%d",
		s.cfg.PublicServerURL, leadID, campaignID)

	callSid, err := exotelClient.InitiateHumanCall(r.Context(), req.AgentPhone, lead.Phone, statusCallback)
	if err != nil {
		s.logger.Sugar().Errorw("humanCallLead", "campaign_id", campaignID, "lead_id", leadID, "err", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("dial failed: %v", err))
		return
	}

	// Log the call so StatusCallback can look it up by call_sid.
	if _, dbErr := s.db.SaveCallLog(leadID, campaignID, ac.OrgID, callSid, "exotel-human", lead.Phone, "initiated"); dbErr != nil {
		s.logger.Sugar().Warnw("humanCallLead: SaveCallLog failed", "err", dbErr)
	}

	// Insert a call_transcripts row (empty transcript, labelled as human call) so the
	// recording appears in the Call Log tab as soon as it is downloaded.
	transcriptStub := `[{"role":"system","content":"Human call — agent bridged to customer via Exotel"}]`
	if transcriptID, tErr := s.db.SaveCallTranscript(leadID, campaignID, ac.OrgID, transcriptStub, "", "", 0); tErr == nil {
		s.store.SetRaw(r.Context(), "transcript_id:"+callSid, fmt.Sprintf("%d", transcriptID), 2*time.Hour)
	} else {
		s.logger.Sugar().Warnw("humanCallLead: SaveCallTranscript failed", "err", tErr)
	}

	// Store campaign Exotel creds in Redis keyed by callSid so downloadRecording
	// can authenticate the recording download with the right account.
	credsJSON, _ := json.Marshal(map[string]string{"api_key": creds.APIKey, "api_token": creds.APIToken})
	s.store.SetRaw(r.Context(), "exotel_creds:"+callSid, string(credsJSON), 4*time.Hour)

	// Remember the initiating user's email so the recording can be saved under
	// a per-user folder regardless of whether the download is triggered from
	// the initial poll or a later StatusCallback re-poll.
	if ac.Email != "" {
		s.store.SetRaw(r.Context(), "user_email:"+callSid, ac.Email, 4*time.Hour)
	}

	// Poll Exotel's Recordings API in the background — StatusCallback does not
	// reliably include RecordingUrl for two-party calls, so we fetch directly.
	capturedCreds := creds
	capturedCallSid := callSid
	capturedLeadID := leadID
	capturedCampaignID := campaignID
	capturedOrgID := ac.OrgID
	go s.pollHumanCallRecording(capturedCallSid, capturedCreds.APIKey, capturedCreds.APIToken,
		capturedCreds.AccountSID, capturedCreds.CallerID, capturedCreds.AppID,
		capturedCreds.Region, capturedCreds.Subdomain,
		capturedLeadID, capturedCampaignID, capturedOrgID, 30*time.Second)

	writeJSON(w, http.StatusOK, map[string]string{"call_sid": callSid, "status": "dialing"})
}

// pollHumanCallRecording polls Exotel's Recordings API every 2 minutes for up
// to 30 minutes, downloads the recording once available, and saves it to both
// call_logs and call_transcripts so it appears in the Call Log UI.
//
// initialWait is how long to sleep before the first attempt. Pass 2*time.Minute
// when starting at dial time (call not yet connected); pass a shorter value
// (e.g. 30s) when re-triggering from a StatusCallback after the call is done.
//
// This is needed because Exotel does not reliably include RecordingUrl in the
// StatusCallback for two-party (From+To) calls.
func (s *Server) pollHumanCallRecording(callSid, apiKey, apiToken, accountSID, callerID, appID, region, subdomain string, leadID, campaignID, orgID int64, initialWait time.Duration) {
	client := dial.NewExotelClient(apiKey, apiToken, accountSID, callerID, appID, "", region, subdomain)
	ctx := context.Background()

	time.Sleep(initialWait)

	for attempt := 1; attempt <= 14; attempt++ {
		recURL, err := client.FetchRecordingURL(ctx, callSid)
		if err != nil {
			s.logger.Warn("pollHumanCallRecording: FetchRecordingURL error",
				zap.String("call_sid", callSid), zap.Int("attempt", attempt), zap.Error(err))
		} else if recURL != "" {
			s.logger.Info("pollHumanCallRecording: recording found",
				zap.String("call_sid", callSid), zap.Int("attempt", attempt), zap.String("url", recURL))
			s.downloadAndSaveHumanRecording(ctx, callSid, recURL, apiKey, apiToken, leadID, campaignID)
			return
		}
		// Not ready yet — wait 30s before retrying.
		time.Sleep(30 * time.Second)
	}
	s.logger.Warn("pollHumanCallRecording: gave up waiting for recording",
		zap.String("call_sid", callSid))
}

// downloadAndSaveHumanRecording downloads a recording URL using the campaign's
// Exotel credentials and updates both call_logs and call_transcripts.
func (s *Server) downloadAndSaveHumanRecording(ctx context.Context, callSid, recordingURL, apiKey, apiToken string, leadID, campaignID int64) {
	parsedURL, err := url.Parse(recordingURL)
	if err != nil {
		s.logger.Warn("downloadAndSaveHumanRecording: invalid URL", zap.Error(err))
		return
	}
	if parsedURL.User == nil && apiKey != "" {
		parsedURL.User = url.UserPassword(apiKey, apiToken)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Get(parsedURL.String())
	if err != nil {
		s.logger.Warn("downloadAndSaveHumanRecording: download failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Warn("downloadAndSaveHumanRecording: HTTP error",
			zap.Int("status", resp.StatusCode))
		return
	}

	ext := ".mp3"
	if strings.Contains(resp.Header.Get("Content-Type"), "wav") {
		ext = ".wav"
	}
	filename := fmt.Sprintf("recording_%s%s", callSid, ext)

	// Read body into memory so we can either upload to S3 or write to disk.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Warn("downloadAndSaveHumanRecording: read body failed", zap.Error(err))
		return
	}

	// Try to place the recording in the initiating user's folder + campaign folder.
	userDir := ""
	campaignDir := ""
	if raw, ok := s.store.GetRaw(ctx, "user_email:"+callSid); ok && raw != "" {
		userDir = sanitizeEmailForPath(raw)
	}
	if campaignID > 0 {
		if c, err := s.db.GetCampaignByID(campaignID); err == nil && c != nil {
			campaignDir = sanitizeEmailForPath(c.Name)
		}
	}

	var savedURL string

	if s.s3 != nil {
		// Upload to S3 and use the public URL.
		s3Key := "recordings/" + filename
		if userDir != "" {
			s3Key = "recordings/" + userDir + "/" + filename
			if campaignDir != "" {
				s3Key = "recordings/" + userDir + "/" + campaignDir + "/" + filename
			}
		}
		publicURL, err := s.s3.UploadPublic(ctx, s3Key, data)
		if err != nil {
			s.logger.Warn("downloadAndSaveHumanRecording: S3 upload failed", zap.Error(err))
			// Fall through to local save below.
		} else {
			savedURL = publicURL
			s.logger.Info("downloadAndSaveHumanRecording: uploaded to S3", zap.String("url", publicURL))
		}
	}

	if savedURL == "" {
		// Local fallback.
		baseDir := s.cfg.RecordingsDir
		urlPrefix := "/api/recordings/"
		if userDir != "" {
			baseDir = filepath.Join(baseDir, userDir)
			urlPrefix = "/api/recordings/" + userDir + "/"
			if campaignDir != "" {
				baseDir = filepath.Join(baseDir, campaignDir)
				urlPrefix = urlPrefix + campaignDir + "/"
			}
		}
		destPath := filepath.Join(baseDir, filename)
		_ = os.MkdirAll(baseDir, 0755)
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			s.logger.Warn("downloadAndSaveHumanRecording: write failed", zap.Error(err))
			return
		}
		savedURL = urlPrefix + filename
	}

	localURL := savedURL

	// Update call_logs
	if err := s.db.UpdateCallLogRecordingURL(callSid, localURL); err != nil {
		s.logger.Warn("downloadAndSaveHumanRecording: UpdateCallLogRecordingURL", zap.Error(err))
	}

	// Update call_transcripts — find the stub row we created at call initiation time.
	if err := s.db.UpdateHumanCallTranscriptRecording(leadID, campaignID, callSid, localURL); err != nil {
		s.logger.Warn("downloadAndSaveHumanRecording: UpdateHumanCallTranscriptRecording", zap.Error(err))
	}

	s.logger.Info("downloadAndSaveHumanRecording: saved",
		zap.String("call_sid", callSid), zap.String("file", filename))
}

// ── RBAC helpers ─────────────────────────────────────────────────────────────

// canViewCampaign decides whether the authenticated user may see a campaign.
// Admins see every org campaign; Team Leaders see campaigns assigned to
// themselves or their managed Agents; Agents see only their own assignments.
// Super-admins bypass org scoping and see every campaign.
func (s *Server) canViewCampaign(ac AuthClaims, campaignID int64) bool {
	campaign, err := s.db.GetCampaignByID(campaignID)
	if err != nil || campaign == nil {
		return false
	}
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		return false
	}
	if s.isSuperAdmin(ac.Email) || user.Role == db.RoleAdmin {
		return true
	}
	if campaign.OrgID != ac.OrgID {
		return false
	}
	if user.Role == db.RoleTeamLeader {
		ok, err := s.db.IsCampaignAssignedToManager(campaignID, user.ID)
		return err == nil && ok
	}
	if user.Role == db.RoleAgent {
		ok, err := s.db.IsCampaignAssignedToUser(campaignID, user.ID)
		return err == nil && ok
	}
	return false
}

// requireCampaignView parses the campaign ID from the URL, verifies the
// campaign exists in the user's org, and enforces RBAC visibility. It returns
// the campaign on success; on failure it writes an HTTP error and returns nil.
// Super-admins bypass the org scoping check so they can inspect any campaign.
func (s *Server) requireCampaignView(w http.ResponseWriter, r *http.Request) *db.Campaign {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	c, err := s.db.GetCampaignByID(id)
	if err != nil {
		s.logger.Sugar().Errorw("requireCampaignView", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if c == nil || (c.OrgID != ac.OrgID && !s.isSuperAdmin(ac.Email)) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return nil
	}
	if !s.canViewCampaign(ac, c.ID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return nil
	}
	return c
}

// leadAccessExecIDs returns the executive_id values a user may access inside a
// campaign. Admins see all leads (apply=false). Agents see unassigned leads
// (executive_id 0/NULL) plus leads assigned to them. Team Leaders see
// unassigned leads plus leads assigned to themselves or their managed Agents.
func (s *Server) leadAccessExecIDs(ac AuthClaims) ([]int64, bool, error) {
	if s.isSuperAdmin(ac.Email) {
		return nil, false, nil
	}
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil {
		return nil, false, err
	}
	if user == nil || user.Role == db.RoleAdmin {
		return nil, false, nil
	}

	ids := []int64{0}
	switch user.Role {
	case db.RoleTeamLeader:
		managed, err := s.db.GetManagedUserIDs(user.ID)
		if err != nil {
			return nil, true, err
		}
		ids = append(ids, user.ID)
		ids = append(ids, managed...)
	case db.RoleAgent:
		ids = append(ids, user.ID)
	}
	return ids, true, nil
}

// canAccessLead checks whether the authenticated user may view/dial a specific
// lead. It verifies org ownership and executive_id assignment.
func (s *Server) canAccessLead(ac AuthClaims, leadID int64) bool {
	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		return false
	}
	if lead.OrgID != ac.OrgID {
		return false
	}
	allowed, apply, err := s.leadAccessExecIDs(ac)
	if err != nil || !apply {
		return !apply
	}
	for _, id := range allowed {
		if id == lead.ExecutiveID {
			return true
		}
	}
	return false
}

// requireLeadAccess fetches a lead and verifies the user can access it (org +
// executive_id). It returns the lead on success, or writes a 404 and returns nil.
func (s *Server) requireLeadAccess(w http.ResponseWriter, r *http.Request, leadID int64) *db.Lead {
	ac := getAuth(r)
	lead, err := s.db.GetLeadByID(leadID)
	if err != nil {
		s.logger.Sugar().Errorw("requireLeadAccess", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if lead == nil || lead.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "lead not found")
		return nil
	}
	allowed, apply, err := s.leadAccessExecIDs(ac)
	if err != nil {
		s.logger.Sugar().Errorw("requireLeadAccess", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if apply {
		found := false
		for _, id := range allowed {
			if id == lead.ExecutiveID {
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, "lead not found")
			return nil
		}
	}
	return lead
}

// requireTranscriptAccess fetches a transcript and verifies the caller may access
// it. It uses lead-level isolation when the transcript has a lead_id, otherwise
// falls back to org ownership. Returns the transcript on success, or writes a
// 404 and returns nil.
func (s *Server) requireTranscriptAccess(w http.ResponseWriter, r *http.Request, transcriptID int64) *db.Transcript {
	ac := getAuth(r)
	t, err := s.db.GetTranscriptByID(transcriptID)
	if err != nil {
		s.logger.Sugar().Errorw("requireTranscriptAccess", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "transcript not found")
		return nil
	}
	if t.LeadID > 0 {
		if !s.canAccessLead(ac, t.LeadID) {
			writeError(w, http.StatusNotFound, "transcript not found")
			return nil
		}
	} else if t.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "transcript not found")
		return nil
	}
	return t
}

// ── POST /api/campaigns/{id}/assign-users ────────────────────────────────────

// @Summary     Assign users to campaign
// @Description Replaces the set of dashboard users assigned to a campaign. Admin only.
// @Tags        campaigns
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64                  true  "Campaign ID"
// @Param       body  body  object{user_ids=[]int64}  true  "User IDs to assign"
// @Success     200   {object}  BoolResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id}/assign-users [post]
func (s *Server) assignCampaignUsers(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	campaign, err := s.db.GetCampaignByID(campaignID)
	if err != nil {
		s.logger.Sugar().Errorw("assignCampaignUsers", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if campaign == nil || campaign.OrgID != ac.OrgID {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	var body struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Reject any user IDs that do not belong to the caller's org.
	if len(body.UserIDs) > 0 {
		seen := make(map[int64]bool, len(body.UserIDs))
		uniqueIDs := make([]int64, 0, len(body.UserIDs))
		for _, uid := range body.UserIDs {
			if uid <= 0 || seen[uid] {
				continue
			}
			seen[uid] = true
			uniqueIDs = append(uniqueIDs, uid)
		}
		body.UserIDs = uniqueIDs

		matchCount, err := s.db.CountUsersByOrgAndIDs(ac.OrgID, body.UserIDs)
		if err != nil {
			s.logger.Sugar().Errorw("assignCampaignUsers", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if int(matchCount) != len(body.UserIDs) {
			writeError(w, http.StatusBadRequest, "one or more users do not belong to this org")
			return
		}
	}

	caller, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || caller == nil {
		writeError(w, http.StatusInternalServerError, "could not resolve caller")
		return
	}

	// Persist the assignment replacement.
	if err := s.db.AssignCampaignToUsers(campaignID, body.UserIDs, caller.ID); err != nil {
		s.logger.Sugar().Errorw("assignCampaignUsers", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Notify newly assigned users in real time.
	for _, uid := range body.UserIDs {
		if uid <= 0 {
			continue
		}
		title := "Campaign assigned"
		bodyText := fmt.Sprintf("You have been assigned to %s", campaign.Name)
		payload := fmt.Sprintf(`{"campaign_id":%d,"campaign_name":%q}`, campaignID, campaign.Name)
		if nid, err := s.db.CreateNotification(uid, "campaign_assigned", title, bodyText, payload); err == nil {
			if s.store != nil {
				n := db.Notification{
					ID:        nid,
					UserID:    uid,
					Type:      "campaign_assigned",
					Title:     title,
					Body:      bodyText,
					Payload:   payload,
					IsRead:    false,
					CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				}
				if b, jerr := json.Marshal(n); jerr == nil {
					s.store.Publish(r.Context(), fmt.Sprintf("user-notifications:%d", uid), string(b))
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── GET /api/campaigns/{id}/assigned-users ───────────────────────────────────

// @Summary     List assigned users
// @Description Returns the user IDs currently assigned to a campaign. Visible to any user who can view the campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  int64  true  "Campaign ID"
// @Success     200  {object}  object{user_ids=[]int64}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id}/assigned-users [get]
func (s *Server) getCampaignAssignedUsers(w http.ResponseWriter, r *http.Request) {
	campaign := s.requireCampaignView(w, r)
	if campaign == nil {
		return
	}
	ids, err := s.db.GetAssignedUserIDsForCampaign(campaign.ID)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignAssignedUsers", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_ids": ids})
}
