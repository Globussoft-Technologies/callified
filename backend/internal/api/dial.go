package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
)

// dialLead initiates an immediate call to a specific lead.
// POST /api/dial/{lead_id}
func (s *Server) dialLead(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseID(r, "lead_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lead_id")
		return
	}

	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}

	var body struct {
		CampaignID int64 `json:"campaign_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	vs, _ := s.db.GetCampaignVoiceSettings(body.CampaignID)
	ac := getAuth(r)

	data := dial.CallData{
		LeadID:      lead.ID,
		LeadName:    lead.FirstName + " " + lead.LastName,
		LeadPhone:   lead.Phone,
		CampaignID:  body.CampaignID,
		OrgID:       ac.OrgID,
		Interest:    lead.Interest,
		TTSProvider: vs.TTSProvider,
		TTSVoiceID:  vs.TTSVoiceID,
		TTSLanguage: vs.TTSLanguage,
	}

	if _, err := s.initiator.Initiate(r.Context(), data); err != nil {
		s.logger.Warn("dialLead: initiate failed",
			zap.Int64("lead_id", leadID), zap.Error(err))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"dialed": true})
}

// campaignDialLead dials a specific lead within a campaign context.
// POST /api/campaigns/{id}/dial/{lead_id}
func (s *Server) campaignDialLead(w http.ResponseWriter, r *http.Request) {
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

	lead, err := s.db.GetLeadByID(leadID)
	if err != nil || lead == nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}

	vs, _ := s.db.GetCampaignVoiceSettings(campaignID)
	ac := getAuth(r)

	data := dial.CallData{
		LeadID:      lead.ID,
		LeadName:    lead.FirstName + " " + lead.LastName,
		LeadPhone:   lead.Phone,
		CampaignID:  campaignID,
		OrgID:       ac.OrgID,
		Interest:    lead.Interest,
		TTSProvider: vs.TTSProvider,
		TTSVoiceID:  vs.TTSVoiceID,
		TTSLanguage: vs.TTSLanguage,
	}

	if _, err := s.initiator.Initiate(r.Context(), data); err != nil {
		s.logger.Warn("campaignDialLead: initiate failed",
			zap.Int64("campaign_id", campaignID), zap.Int64("lead_id", leadID), zap.Error(err))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"dialed": true})
}

// campaignDialAll queues a campaign's leads for sequential dialing with a
// 30s gap between calls. Ports Python's dial_routes.py:307-377 exactly.
//
//   - ?force=true  → dial EVERY lead (status-agnostic); used by the
//     "Dial All (N)" button to redial leads already in non-new states.
//   - no force     → dial only leads whose status is "new"/"New"; used by
//     the "Dial All New (N)" button. Matches Python's default behaviour.
//
// Previous Go impl hard-coded a status exclusion list (skipping Calling /
// Completed / DND) and ignored `force` entirely. That meant the "Dial All"
// button silently queued zero calls when every lead had already been dialed
// once — which is exactly the reported symptom.
//
// POST /api/campaigns/{id}/dial-all
func (s *Server) campaignDialAll(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	force := r.URL.Query().Get("force") == "true"

	leads, err := s.db.GetCampaignLeads(campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list leads")
		return
	}

	// Python's filter: when not forced, only dial leads with status "new"
	// (case-insensitive). "force=true" bypasses the filter and dials every
	// lead regardless — matches the frontend contract.
	dialable := make([]db.CampaignLead, 0, len(leads))
	for _, l := range leads {
		if force {
			dialable = append(dialable, l)
			continue
		}
		st := strings.ToLower(strings.TrimSpace(l.Status))
		if st == "" || st == "new" {
			dialable = append(dialable, l)
		}
	}
	if len(dialable) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "error",
			"message": "No leads to dial",
			"queued":  0,
		})
		return
	}

	vs, _ := s.db.GetCampaignVoiceSettings(campaignID)
	ac := getAuth(r)

	// Detach from the HTTP request's context — the queue runs for minutes
	// after the HTTP response returns. Using r.Context() would cancel every
	// pending dial the moment the response flushes.
	ctx := context.Background()
	queue := make([]dial.CallData, 0, len(dialable))
	for _, l := range dialable {
		queue = append(queue, dial.CallData{
			LeadID:      l.ID,
			LeadName:    l.FirstName + " " + l.LastName,
			LeadPhone:   l.Phone,
			CampaignID:  campaignID,
			OrgID:       ac.OrgID,
			Interest:    l.Interest,
			TTSProvider: vs.TTSProvider,
			TTSVoiceID:  vs.TTSVoiceID,
			TTSLanguage: vs.TTSLanguage,
		})
	}

	go func() {
		verb := "new leads"
		if force {
			verb = "leads"
		}
		s.store.EmitCampaignEvent(ctx, campaignID, "Campaign", "",
			"started", fmt.Sprintf("Dialing %d %s", len(queue), verb))
		for i, d := range queue {
			if i > 0 {
				time.Sleep(30 * time.Second)
			}
			if d.OrgID > 0 {
				if isDND, _ := s.db.IsDNDNumber(d.OrgID, d.LeadPhone); isDND {
					s.store.EmitCampaignEvent(ctx, campaignID, d.LeadName, d.LeadPhone,
						"dnd", "on DND list")
					continue
				}
			}
			if _, err := s.initiator.Initiate(ctx, d); err != nil {
				s.logger.Warn("campaignDialAll: lead failed",
					zap.Int64("lead_id", d.LeadID), zap.Error(err))
				// Initiator already emits `failed` on error — no duplicate.
			}
		}
		s.store.EmitCampaignEvent(ctx, campaignID, "Campaign", "",
			"finished", fmt.Sprintf("Dial queue complete (%d leads)", len(queue)))
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Dialing %d leads (30s gap between calls)", len(queue)),
		"queued":  len(queue),
	})
}

// campaignRedialFailed re-dials all leads in the campaign that have a
// "Call Failed*" status. Matches Python's dial_routes.py:239-287 behaviour:
//   - sequential, not parallel (30s gap between calls — prevents carrier spam
//     flags and matches the confirm-dialog the frontend shows users)
//   - emits campaign-level "started" event + per-lead events to the Live
//     Campaign Activity feed
//   - skips DND numbers with a `dnd_skipped` event
//   - returns a user-friendly `message` that the frontend surfaces via alert()
//
// POST /api/campaigns/{id}/redial-failed
func (s *Server) campaignRedialFailed(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	leads, err := s.db.GetFailedLeadsInCampaign(campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list failed leads")
		return
	}
	if len(leads) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "error",
			"message": "No failed leads to redial",
			"queued":  0,
		})
		return
	}

	vs, _ := s.db.GetCampaignVoiceSettings(campaignID)
	ac := getAuth(r)

	// Copy slice into a simple, independently-owned value before handing it
	// to the background goroutine — r.Context() cancels when this handler
	// returns, but the redial queue runs for minutes. Use a detached ctx.
	ctx := context.Background()
	queue := make([]dial.CallData, 0, len(leads))
	for _, lead := range leads {
		queue = append(queue, dial.CallData{
			LeadID:      lead.ID,
			LeadName:    lead.FirstName + " " + lead.LastName,
			LeadPhone:   lead.Phone,
			CampaignID:  campaignID,
			OrgID:       ac.OrgID,
			Interest:    lead.Interest,
			TTSProvider: vs.TTSProvider,
			TTSVoiceID:  vs.TTSVoiceID,
			TTSLanguage: vs.TTSLanguage,
		})
	}

	go func() {
		s.store.EmitCampaignEvent(ctx, campaignID, "Campaign", "",
			"started", fmt.Sprintf("Redialing %d failed leads", len(queue)))
		for i, d := range queue {
			if i > 0 {
				time.Sleep(30 * time.Second)
			}
			// DND check mirrors Python — skip and log to the feed so users
			// can see why the number was held back.
			if d.OrgID > 0 {
				if isDND, _ := s.db.IsDNDNumber(d.OrgID, d.LeadPhone); isDND {
					s.store.EmitCampaignEvent(ctx, campaignID, d.LeadName, d.LeadPhone,
						"dnd", "on DND list")
					continue
				}
			}
			if _, err := s.initiator.Initiate(ctx, d); err != nil {
				s.logger.Warn("campaignRedialFailed: lead failed",
					zap.Int64("lead_id", d.LeadID), zap.Error(err))
				// initiator.Initiate already emits a `failed` event on errors,
				// so no duplicate emit needed here.
			}
		}
		s.store.EmitCampaignEvent(ctx, campaignID, "Campaign", "",
			"finished", fmt.Sprintf("Redial queue complete (%d leads)", len(queue)))
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Redialing %d failed leads (30s gap between calls)", len(queue)),
		"queued":  len(queue),
	})
}
