package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/scheduled-calls ──────────────────────────────────────────────────

// @Summary     List scheduled calls
// @Description Returns scheduled calls for the org. Supports filtering by mode,
//              status and due time. Requires Admin role.
// @Tags        scheduled-calls
// @Produce     json
// @Security    BearerAuth
// @Param       mode    query     string  false  "Filter by mode: ai | manual"
// @Param       status  query     string  false  "Filter by status: pending | dialing | completed | failed | cancelled"
// @Param       due     query     bool    false  "If true, return only calls due now (scheduled_at <= NOW() + 30s)"
// @Success     200     {array}   object
// @Failure     401     {object}  ErrorResponse
// @Failure     403     {object}  ErrorResponse
// @Failure     500     {object}  ErrorResponse
// @Router      /api/scheduled-calls [get]
func (s *Server) listScheduledCalls(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	q := r.URL.Query()

	var calls []db.ScheduledCall
	var err error

	if q.Get("due") == "true" {
		calls, err = s.db.GetDueManualScheduledCalls(ac.OrgID, 30)
	} else {
		calls, err = s.db.GetScheduledCallsByOrg(ac.OrgID)
	}
	if err != nil {
		s.logger.Sugar().Errorw("listScheduledCalls", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	modeFilter := q.Get("mode")
	statusFilter := q.Get("status")
	if modeFilter != "" || statusFilter != "" {
		filtered := calls[:0]
		for _, c := range calls {
			if modeFilter != "" && c.Mode != modeFilter {
				continue
			}
			if statusFilter != "" && c.Status != statusFilter {
				continue
			}
			filtered = append(filtered, c)
		}
		calls = filtered
	}

	writeJSON(w, http.StatusOK, emptyJSON(calls))
}

// ── POST /api/scheduled-calls ─────────────────────────────────────────────────

// @Summary     Schedule a call
// @Description Schedules a future outbound or manual call for a lead. Requires Admin role.
// @Tags        scheduled-calls
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{lead_id=int64,campaign_id=int64,scheduled_at=string,notes=string,mode=string,executive_id=int64}  true  "scheduled_at: RFC3339 or YYYY-MM-DD HH:MM:SS"
// @Success     201   {object}  IDResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     409   {object}  ErrorResponse  "lead is on DND list"
// @Failure     500   {object}  ErrorResponse
// @Router      /api/scheduled-calls [post]
func (s *Server) createScheduledCall(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		LeadID      int64  `json:"lead_id"`
		CampaignID  int64  `json:"campaign_id"`
		ScheduledAt string `json:"scheduled_at"`
		Notes       string `json:"notes"`
		Mode        string `json:"mode"`
		ExecutiveID int64  `json:"executive_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.LeadID == 0 || body.ScheduledAt == "" {
		writeError(w, http.StatusBadRequest, "lead_id and scheduled_at required")
		return
	}

	var scheduledAt time.Time
	var parseErr error
	scheduledAt, parseErr = time.Parse(time.RFC3339, body.ScheduledAt)
	if parseErr != nil {
		scheduledAt, parseErr = time.Parse("2006-01-02 15:04:05", body.ScheduledAt)
	}
	if parseErr != nil {
		writeError(w, http.StatusBadRequest, "invalid scheduled_at — use RFC3339 or YYYY-MM-DD HH:MM:SS")
		return
	}
	if scheduledAt.Before(time.Now().Add(-1 * time.Minute)) {
		writeError(w, http.StatusBadRequest, "scheduled_at must be in the future")
		return
	}

	lead, leadErr := s.db.GetLeadByID(body.LeadID)
	if leadErr != nil || lead == nil {
		writeError(w, http.StatusBadRequest, "lead not found")
		return
	}
	if isDND, dndErr := s.db.IsDNDNumber(ac.OrgID, lead.Phone); dndErr == nil && isDND {
		writeError(w, http.StatusConflict,
			"This number is on the DND list. Remove it from DND before scheduling.")
		return
	}

	mode := body.Mode
	if mode == "" {
		mode = "ai"
	}
	executiveID := body.ExecutiveID
	if mode == "manual" && executiveID == 0 && lead.ExecutiveID != 0 {
		executiveID = lead.ExecutiveID
	}

	id, err := s.db.CreateScheduledCall(ac.OrgID, body.LeadID, body.CampaignID, executiveID, scheduledAt, body.Notes, mode)
	if err != nil {
		s.logger.Sugar().Errorw("createScheduledCall", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ── DELETE /api/scheduled-calls/{id} ─────────────────────────────────────────

// @Summary     Cancel scheduled call
// @Description Cancels a pending scheduled call. Requires Admin role.
// @Tags        scheduled-calls
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Scheduled Call ID"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/scheduled-calls/{id} [delete]
func (s *Server) cancelScheduledCall(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	cancelled, err := s.db.CancelScheduledCall(ac.OrgID, id)
	if err != nil {
		s.logger.Sugar().Errorw("cancelScheduledCall", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !cancelled {
		writeError(w, http.StatusNotFound, "not found or already processed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}
