package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/llm"
)

// reportResponse is the shape returned by GET /api/calls/{call_id}/report.
type reportResponse struct {
	TranscriptID    int64           `json:"transcript_id"`
	LeadID          int64           `json:"lead_id"`
	CampaignID      int64           `json:"campaign_id"`
	OrgID           int64           `json:"org_id"`
	LeadFirstName   string          `json:"lead_first_name"`
	LeadLastName    string          `json:"lead_last_name"`
	LeadPhone       string          `json:"lead_phone"`
	CampaignName    string          `json:"campaign_name"`
	Transcript      json.RawMessage `json:"transcript"`
	RecordingURL    string          `json:"recording_url"`
	TTSLanguage     string          `json:"tts_language"`
	CallDurationS   float64         `json:"call_duration_s"`
	CostEstimate    float64         `json:"cost_estimate_paise"`
	Review          *db.CallReview   `json:"review,omitempty"`
	AnalysisPending bool            `json:"analysis_pending"`
	CreatedAt       string          `json:"created_at"`
}

// @Summary     Get call report
// @Description Returns a transcript, lead/campaign context, cost estimate and review for a single call. If no review exists, analysis is triggered asynchronously and a 202 with analysis_pending=true is returned.
// @Tags        calls
// @Produce     json
// @Security    BearerAuth
// @Param       call_id  path  int64  true  "Call (transcript) ID"
// @Success     200  {object}  reportResponse
// @Success     202  {object}  reportResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/calls/{call_id}/report [get]
func (s *Server) getCallReport(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	callID, err := strconv.ParseInt(r.PathValue("call_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid call id")
		return
	}

	report, err := s.db.GetCallReport(callID)
	if err != nil {
		s.logger.Sugar().Errorw("getCallReport", "err", err, "call_id", callID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if report == nil {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	// Scope to the caller's org unless super-admin.
	if report.OrgID != ac.OrgID && !s.isSuperAdmin(ac.Email) {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	// Lead-level isolation: if the transcript has a lead, the user must be able to access it.
	if report.LeadID > 0 && !s.canAccessLead(ac, report.LeadID) {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	review, err := s.db.GetCallReviewByTranscript(callID)
	if err != nil {
		s.logger.Sugar().Errorw("getCallReport: review lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	report.Review = review

	resp := reportResponse{
		TranscriptID:    report.TranscriptID,
		LeadID:          report.LeadID,
		CampaignID:      report.CampaignID,
		OrgID:           report.OrgID,
		LeadFirstName:   report.LeadFirstName,
		LeadLastName:    report.LeadLastName,
		LeadPhone:       report.LeadPhone,
		CampaignName:    report.CampaignName,
		Transcript:      report.Transcript,
		RecordingURL:    report.RecordingURL,
		TTSLanguage:     report.TTSLanguage,
		CallDurationS:   report.CallDurationS,
		CostEstimate:    report.CostEstimate,
		Review:          report.Review,
		AnalysisPending: false,
		CreatedAt:       report.CreatedAt,
	}

	if report.Review == nil && s.recordingSvc != nil {
		// Trigger asynchronous analysis and return 202 Accepted.
		go s.triggerReportAnalysis(r.Context(), callID)
		resp.AnalysisPending = true
		writeJSON(w, http.StatusAccepted, resp)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) triggerReportAnalysis(ctx context.Context, callID int64) {
	report, err := s.db.GetCallReport(callID)
	if err != nil || report == nil {
		s.logger.Warn("triggerReportAnalysis: could not load report", zap.Int64("call_id", callID), zap.Error(err))
		return
	}
	var history []llm.ChatMessage
	if len(report.Transcript) > 0 {
		_ = json.Unmarshal(report.Transcript, &history)
	}
	if len(history) == 0 {
		s.logger.Info("triggerReportAnalysis: empty transcript, nothing to analyze", zap.Int64("call_id", callID))
		return
	}
	// Run analysis with a generous timeout; this is fire-and-forget from the HTTP path.
	analysisCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	analysis, err := s.recordingSvc.AnalyzeCall(analysisCtx, history)
	if err != nil || analysis == nil {
		s.logger.Warn("triggerReportAnalysis: analysis failed", zap.Int64("call_id", callID), zap.Error(err))
		return
	}
	review := &db.CallReview{
		TranscriptID:                callID,
		OrgID:                       report.OrgID,
		QualityScore:                analysis.QualityScore,
		Sentiment:                   analysis.Sentiment,
		AppointmentBooked:           analysis.AppointmentBooked,
		FailureReason:               analysis.FailureReason,
		WhatWentWell:                analysis.WhatWentWell,
		WhatWentWrong:               analysis.WhatWentWrong,
		Summary:                     analysis.Summary,
		Insights:                    analysis.Insights,
		PromptImprovementSuggestion: analysis.PromptImprovementSuggestion,
	}
	if err := s.db.SaveCallReview(review); err != nil {
		s.logger.Warn("triggerReportAnalysis: save review failed", zap.Int64("call_id", callID), zap.Error(err))
	}
}
