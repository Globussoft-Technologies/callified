package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// ── GET /api/external/transcripts ─────────────────────────────────────────────
//
// Bulk export of every campaign in the caller's org, with each campaign's
// leads nested underneath, and each lead's transcripts (plus the Gemini-
// generated conclusion) nested under that. Designed for partner / integration
// teams that want a single call to pull everything instead of walking the
// per-resource endpoints.
//
// Auth: API key via X-API-Key header, or standard Bearer JWT.
// Org-scoped through ac.OrgID so a partner can only see their own org's data.
//
// Query params:
//
//	?campaign_id=<id>  — restrict to one campaign. Omit to get all campaigns
//	                     in the org.
//
// Response shape:
//
//	[
//	  {
//	    "campaign_id":   5,
//	    "campaign_name": "Whatsapp campaign",
//	    "channel":       "WhatsApp",
//	    "status":        "active",
//	    "created_at":    "2026-04-12 09:00:00",
//	    "totals":        { "total": 2, "called": 2, "qualified": 0, "booked": 0 },
//	    "leads": [
//	      {
//	        "lead_id":   12,
//	        "lead_name": "Harsha",
//	        "phone":     "9177007429",
//	        "status":    "Completed",
//	        "calls": [
//	          {
//	            "id":            259,
//	            "duration_s":    56.7,
//	            "tts_language":  "en",
//	            "created_at":    "2026-04-28 10:01:08",
//	            "recording_url": "/api/recordings/...wav",
//	            "transcript":    [ {"role":"AI","text":"..."}, {"role":"User","text":"..."} ],
//	            "conclusion":    { "quality_score": 4.0, "sentiment": "positive", ... } | null
//	          }
//	        ]
//	      }
//	    ]
//	  }
//	]
func (s *Server) getExternalTranscripts(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)

	// Optional ?campaign_id=N filter. Invalid values are ignored rather than
	// 400'd so the partner doesn't have to special-case "give me everything".
	var filterCampaignID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("campaign_id")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			filterCampaignID = n
		}
	}

	campaigns, err := s.db.GetCampaignsByOrg(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getExternalTranscripts: campaigns lookup", "err", err, "org_id", ac.OrgID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]map[string]any, 0, len(campaigns))
	for _, c := range campaigns {
		if filterCampaignID > 0 && c.ID != filterCampaignID {
			continue
		}

		stats, _ := s.db.GetCampaignStats(c.ID)
		leads, err := s.db.GetCampaignLeads(c.ID)
		if err != nil {
			s.logger.Sugar().Warnw("getExternalTranscripts: leads lookup", "err", err, "campaign_id", c.ID)
			leads = nil
		}

		leadRows := make([]map[string]any, 0, len(leads))
		for _, l := range leads {
			transcripts, err := s.db.GetTranscriptsByLead(l.ID)
			if err != nil {
				s.logger.Sugar().Warnw("getExternalTranscripts: transcripts lookup", "err", err, "lead_id", l.ID)
				continue
			}
			// Filter to this campaign's transcripts only. GetTranscriptsByLead
			// returns rows across all campaigns the lead has touched, which
			// would otherwise show duplicate calls under every campaign.
			calls := make([]map[string]any, 0, len(transcripts))
			for _, t := range transcripts {
				if t.CampaignID != c.ID {
					continue
				}
				var turns []map[string]any
				if len(t.Transcript) > 0 {
					if err := json.Unmarshal(t.Transcript, &turns); err != nil {
						turns = []map[string]any{}
					}
				}
				if turns == nil {
					turns = []map[string]any{}
				}

				var conclusion any
				if review, _ := s.db.GetCallReviewByTranscript(t.ID); review != nil {
					conclusion = map[string]any{
						"quality_score":                 review.QualityScore,
						"sentiment":                     review.Sentiment,
						"appointment_booked":            review.AppointmentBooked,
						"failure_reason":                review.FailureReason,
						"summary":                       review.Summary,
						"what_went_well":                review.WhatWentWell,
						"what_went_wrong":               review.WhatWentWrong,
						"insights":                      review.Insights,
						"prompt_improvement_suggestion": review.PromptImprovementSuggestion,
					}
				}

				calls = append(calls, map[string]any{
					"id":            t.ID,
					"duration_s":    t.CallDurationS,
					"tts_language":  t.TTSLanguage,
					"created_at":    t.CreatedAt,
					"recording_url": t.RecordingURL,
					"transcript":    turns,
					"conclusion":    conclusion,
				})
			}

			leadRows = append(leadRows, map[string]any{
				"lead_id":   l.ID,
				"lead_name": strings.TrimSpace(l.FirstName + " " + l.LastName),
				"phone":     l.Phone,
				"status":    l.Status,
				"calls":     calls,
			})
		}

		out = append(out, map[string]any{
			"campaign_id":   c.ID,
			"campaign_name": c.Name,
			"channel":       c.Channel,
			"status":        c.Status,
			"created_at":    c.CreatedAt,
			"totals": map[string]any{
				"total":     stats.Total,
				"called":    stats.Called,
				"qualified": stats.Qualified,
				"booked":    stats.Appointments,
			},
			"leads": leadRows,
		})
	}

	s.logger.Info("external transcripts export",
		zap.Int64("org_id", ac.OrgID),
		zap.Int("campaigns", len(out)),
		zap.Int64("filter_campaign_id", filterCampaignID),
	)
	writeJSON(w, http.StatusOK, out)
}
