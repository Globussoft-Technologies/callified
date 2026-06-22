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
		campaigns, err = s.db.GetCampaignsByOrg(ac.OrgID)
	}
	if err != nil {
		s.logger.Sugar().Errorw("listCampaigns", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(campaigns))
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
	if c == nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	// Attach fresh stats — best-effort; we don't fail the whole response if
	// the stats query breaks.
	if stats, err := s.db.GetCampaignStats(id); err == nil {
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
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

// @Summary     List campaign leads
// @Description Returns all leads enrolled in a campaign.
// @Tags        campaigns
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Campaign ID"
// @Success     200  {array}   db.CampaignLead
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/campaigns/{id}/leads [get]
func (s *Server) listCampaignLeads(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	leads, err := s.db.GetCampaignLeads(id)
	if err != nil {
		s.logger.Sugar().Errorw("listCampaignLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(leads))
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		LeadIDs []int64 `json:"lead_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.LeadIDs) == 0 {
		writeError(w, http.StatusBadRequest, "lead_ids required")
		return
	}
	added, err := s.db.AddLeadsToCampaign(id, body.LeadIDs)
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
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stats, err := s.db.GetCampaignStats(id)
	if err != nil {
		s.logger.Sugar().Errorw("getCampaignStats", "err", err)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	log, err := s.db.GetCampaignCallLog(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	campaign, err := s.db.GetCampaignByID(id)
	if err != nil || campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	entries, err := s.db.GetCampaignRecordingsExport(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	vs, err := s.db.GetCampaignVoiceSettings(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
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
// @Param       file  formData  file   true  "CSV file (columns: first_name, last_name, phone, source)"
// @Success     200   {object}  object{imported=int,added_to_campaign=int,errors=[]string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/campaigns/{id}/import-csv [post]
func (s *Server) importCampaignLeadsCSV(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
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
	iFirst, iLast, iPhone, iSource := idx("first_name"), idx("last_name"), idx("phone"), idx("source")
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

	// Deduplicate the CSV by phone (first occurrence wins) and skip empty phones.
	var rows []db.LeadImportRow
	seen := make(map[string]bool)
	for _, rec := range records[1:] {
		phone := get(rec, iPhone)
		if phone == "" || seen[phone] {
			continue
		}
		seen[phone] = true
		rows = append(rows, db.LeadImportRow{
			FirstName: get(rec, iFirst), LastName: get(rec, iLast),
			Phone: phone, Source: get(rec, iSource),
		})
	}

	// Find which phones already exist in this org and which leads are already in the campaign.
	var phones []string
	for _, r := range rows {
		phones = append(phones, r.Phone)
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

	// Count how many new leads would actually be linked to the campaign.
	var newRows []db.LeadImportRow
	var existingToAdd int
	for _, r := range rows {
		if id, ok := existing[r.Phone]; ok {
			if !campaignLeadIDs[id] {
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

	writeJSON(w, http.StatusOK, map[string]any{
		"imported":          imported,
		"added_to_campaign": addedToCampaign,
		"errors":            errs,
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	creds, err := s.db.GetCampaignExotelCreds(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var creds db.ExotelCreds
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.db.SaveCampaignExotelCreds(id, creds); err != nil {
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	reviews, err := s.db.GetCallReviewsByCampaign(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	retries, err := s.db.GetRetriesByCampaignWithLead(id)
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
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	insights, err := s.db.GetCampaignCallInsights(id)
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

	creds, err := s.db.GetCampaignExotelCreds(campaignID)
	if err != nil || !creds.IsSet() {
		writeError(w, http.StatusBadRequest, "no Exotel credentials configured for this campaign")
		return
	}

	exotelClient := dial.NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType, creds.Region, creds.Subdomain)

	// StatusCallback delivers recording URL + final status when the call ends.
	ac := getAuth(r)
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

	var savedURL string

	if s.s3 != nil {
		// Upload to S3 and use the public URL.
		s3Key := "recordings/" + filename
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
		destPath := filepath.Join(s.cfg.RecordingsDir, filename)
		_ = os.MkdirAll(s.cfg.RecordingsDir, 0755)
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			s.logger.Warn("downloadAndSaveHumanRecording: write failed", zap.Error(err))
			return
		}
		savedURL = fmt.Sprintf("/api/recordings/%s", filename)
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
