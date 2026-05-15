// Package recording handles end-of-call processing: WAV saving, Gemini analysis,
// call review insertion, DND auto-add, and webhook + WA confirmation dispatch.
// This replaces the gRPC FinalizeCall Python call (Phase 4).
package recording

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/llm"
	"github.com/globussoft/callified-backend/internal/wa"
	"github.com/globussoft/callified-backend/internal/webhook"
)

// SaveRequest contains all data needed to save and analyze one call.
type SaveRequest struct {
	StreamSid   string
	CallSid     string
	LeadID      int64
	CampaignID  int64
	OrgID       int64
	LeadPhone   string
	AgentName   string
	TTSLanguage string // language the call was synthesised in (hi/mr/bn/gu/pa/ta/te/kn/ml/en)
	ChatHistory []llm.ChatMessage
	DurationS   float32
	StereoWav   []byte // nil → no server-side recording
}

// Service handles post-call analysis.
type Service struct {
	database   *db.DB
	llm        *llm.Provider
	dispatcher *webhook.Dispatcher
	cfg        *config.Config
	log        *zap.Logger
}

// New creates a Service.
func New(database *db.DB, llmProvider *llm.Provider, dispatcher *webhook.Dispatcher, cfg *config.Config, log *zap.Logger) *Service {
	return &Service{
		database:   database,
		llm:        llmProvider,
		dispatcher: dispatcher,
		cfg:        cfg,
		log:        log,
	}
}

// SaveAndAnalyze runs the full post-call pipeline asynchronously.
// It is fire-and-forget from the WebSocket handler's perspective — call it in a goroutine.
func (s *Service) SaveAndAnalyze(ctx context.Context, req SaveRequest) {
	// Use a background context so cleanup isn't cancelled when the WS connection closes.
	ctx = context.Background()

	// 1. Save WAV to disk (if recorded server-side).
	recordingURL := ""
	if len(req.StereoWav) > 0 {
		recordingURL = s.saveWAV(req.StreamSid, req.StereoWav)
	}

	// 2. Build transcript turns ([{role,text}, ...]) from chat history.
	//    Mirrors recording_service.py: role mapping model→AI / user→User,
	//    empty-text turns dropped.
	transcriptJSON, turnCount := historyToTranscript(req.ChatHistory)

	// 3. Skip only when there are no turns AND no recording — i.e. truly empty
	//    sessions (immediate disconnect with no audio). When a recording exists
	//    we still persist the row so the call shows up in the Transcripts modal
	//    and the WebM-upload path has a row to attach its URL to. Without this,
	//    calls with audio but no STT/LLM turns silently disappeared from the UI.
	if turnCount == 0 && recordingURL == "" {
		s.log.Info("recording: skipping empty transcript",
			zap.String("stream_sid", req.StreamSid),
			zap.Int("raw_turns", len(req.ChatHistory)))
		return
	}

	// 4. Persist transcript row — same INSERT columns as Python save_call_transcript.
	transcriptID, err := s.database.SaveCallTranscript(req.LeadID, req.CampaignID, req.OrgID, transcriptJSON, recordingURL, req.TTSLanguage, req.DurationS)
	if err != nil {
		s.log.Error("recording: SaveCallTranscript failed", zap.Error(err))
		return
	}
	s.log.Info("recording: transcript saved",
		zap.Int64("transcript_id", transcriptID),
		zap.Int("turn_count", turnCount),
		zap.Float32("duration_s", req.DurationS))

	// 4. Run Gemini analysis (non-critical — log and continue on failure).
	//
	// Skip analysis entirely for very short / one-sided calls: a 2-3 second
	// hang-up or a no-reply greeting has nothing meaningful to analyze, and
	// asking Gemini to score it tends to produce inflated 4/5 + positive
	// because Gemini judges the agent's intro instead of the call outcome.
	// Skipping means no call_reviews row is saved, so the UI's conclusion
	// card naturally stays hidden for these calls.
	userTurns := 0
	for _, m := range req.ChatHistory {
		if strings.EqualFold(m.Role, "user") {
			userTurns++
		}
	}
	shouldAnalyze := s.llm != nil && len(req.ChatHistory) > 0 && req.DurationS >= 10 && userTurns >= 1

	review := &db.CallReview{
		TranscriptID: transcriptID,
		OrgID:        req.OrgID,
		Sentiment:    "neutral",
	}
	analyzed := false
	if shouldAnalyze {
		if a, err := s.analyzeCall(ctx, req.ChatHistory); err != nil {
			s.log.Warn("recording: Gemini analysis failed", zap.Error(err))
		} else {
			review.QualityScore = a.QualityScore
			review.Sentiment = a.Sentiment
			review.AppointmentBooked = a.AppointmentBooked
			review.FailureReason = a.FailureReason
			review.WhatWentWell = a.WhatWentWell
			review.WhatWentWrong = a.WhatWentWrong
			review.Summary = a.Summary
			review.Insights = a.Insights
			review.PromptImprovementSuggestion = a.PromptImprovementSuggestion
			analyzed = true
		}
	} else {
		s.log.Info("recording: skipping Gemini analysis (short/one-sided)",
			zap.Int64("transcript_id", transcriptID),
			zap.Float32("duration_s", req.DurationS),
			zap.Int("user_turns", userTurns))
	}

	// 5. Save call review only when Gemini actually produced commentary.
	// Saving an empty-default row would render in the UI as a misleading
	// "0/5 neutral / no appointment" card with no real conclusion.
	if analyzed {
		if err := s.database.SaveCallReview(review); err != nil {
			s.log.Error("recording: SaveCallReview failed", zap.Error(err))
		}
	}

	// 5b. Deduct call duration from the org's prepaid credit balance.
	// Skipped automatically for web-sim calls (CallSid == "") so only real
	// telephony calls are billed. Idempotent on the call_sid — if the
	// recording pipeline runs twice for the same call (race between Exotel's
	// "completed" callback and the WS-side finalize), the second call is a
	// no-op.
	if req.CallSid != "" && req.DurationS > 0 && req.OrgID > 0 {
		if charge, balance, err := s.database.DeductCallCredits(req.OrgID, req.CallSid, float64(req.DurationS)); err != nil {
			s.log.Warn("recording: DeductCallCredits failed",
				zap.String("call_sid", req.CallSid), zap.Error(err))
		} else if charge > 0 {
			s.log.Info("recording: credits deducted",
				zap.String("call_sid", req.CallSid),
				zap.Int64("charge_paise", charge),
				zap.Int64("balance_after_paise", balance),
				zap.Float32("duration_s", req.DurationS))
		}
	}

	// 6. Auto-DND if clearly negative + "do not call" intent.
	if review.Sentiment == "negative" && req.LeadPhone != "" && containsDNC(req.ChatHistory) {
		if err := s.database.AddDNDNumber(req.OrgID, req.LeadPhone, "auto: negative sentiment + DNC intent"); err != nil {
			s.log.Warn("recording: auto-DND failed", zap.Error(err))
		} else {
			s.log.Info("recording: auto-added to DND", zap.String("phone", req.LeadPhone))
		}
	}

	// 7. Fire call.completed webhook.
	if s.dispatcher != nil {
		s.dispatcher.Dispatch(ctx, req.OrgID, "call.completed", map[string]any{
			"transcript_id":     transcriptID,
			"lead_id":           req.LeadID,
			"campaign_id":       req.CampaignID,
			"duration_s":        req.DurationS,
			"sentiment":         review.Sentiment,
			"appointment_booked": review.AppointmentBooked,
		})
	}

	// 8. Send WA appointment confirmation if appointment was booked.
	if review.AppointmentBooked && req.LeadPhone != "" {
		s.sendAppointmentConfirmation(ctx, req.OrgID, req.LeadPhone, req.AgentName)
	}

	s.log.Info("recording: post-call processing complete",
		zap.Int64("transcript_id", transcriptID),
		zap.String("sentiment", review.Sentiment),
		zap.Bool("appointment_booked", review.AppointmentBooked),
	)
}

// ── WAV saving ────────────────────────────────────────────────────────────────

func (s *Service) saveWAV(streamSid string, data []byte) string {
	if s.cfg.RecordingsDir == "" {
		return ""
	}
	if err := os.MkdirAll(s.cfg.RecordingsDir, 0755); err != nil {
		s.log.Warn("recording: mkdir failed", zap.Error(err))
		return ""
	}
	filename := fmt.Sprintf("%s_%d.wav", sanitize(streamSid), time.Now().UnixMilli())
	path := filepath.Join(s.cfg.RecordingsDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.log.Warn("recording: WriteFile failed", zap.Error(err))
		return ""
	}
	return "/api/recordings/" + filename
}

// ── Gemini analysis ───────────────────────────────────────────────────────────

type analysis struct {
	QualityScore                float64 `json:"quality_score"`
	Sentiment                   string  `json:"sentiment"`
	AppointmentBooked           bool    `json:"appointment_booked"`
	FailureReason               string  `json:"failure_reason"`
	WhatWentWell                string  `json:"what_went_well"`
	WhatWentWrong               string  `json:"what_went_wrong"`
	Summary                     string  `json:"summary"`
	Insights                    string  `json:"insights"`
	PromptImprovementSuggestion string  `json:"prompt_improvement_suggestion"`
}

// Analysis is the exported view of the LLM scoring output. Mirrors `analysis`
// so the API layer can call AnalyzeCall on demand (for the Transcript modal's
// "regenerate conclusion" flow) without importing internal types.
type Analysis = analysis

// AnalyzeCall is the public wrapper around analyzeCall. Used by the API
// layer to (re)generate a call conclusion on demand when the post-call
// pipeline skipped it (short/one-sided call) or the operator hits a
// "Regenerate" button. Keeps the same prompt and parsing as the post-call
// path so the conclusion card stays consistent regardless of who triggered it.
func (s *Service) AnalyzeCall(ctx context.Context, history []llm.ChatMessage) (*Analysis, error) {
	return s.analyzeCall(ctx, history)
}

// Strict prompt: every prose field MUST be populated (never empty, never
// omitted), score MUST be an integer 1-5, and the analysis is always written
// in English regardless of the transcript language.
//
// Crucially, the score must reflect CALL OUTCOME (did the customer engage and
// move toward an appointment?), not just agent technique. A 3-second one-sided
// greeting where the customer never spoke is a FAILED call and must score
// 1-2 with neutral sentiment — regardless of how well the AI delivered its
// intro. Previously Gemini was scoring these 4/5 + positive because it judged
// the agent's performance instead of the call outcome.
const analysisSystemPrompt = `You are a sales call quality analyst scoring CALL OUTCOME, not agent technique. Analyze the provided transcript and return ONLY a JSON object with EVERY field populated in English (no nulls, no empty strings, no omitted keys):

SCORING RULES — apply STRICTLY:
- If the customer never spoke (0 user turns / only the AI delivered a greeting): "quality_score" must be 1 or 2, "sentiment" must be "neutral", "appointment_booked" must be false. The customer's silence is the call outcome — never call this "positive" no matter how well the agent introduced themselves.
- If the customer spoke only 1-2 short words ("hello", "yes", "ok") and the call ended: score 2. Sentiment "neutral".
- If the customer engaged but refused or pushed back: score 2-3. Sentiment "negative" or "annoyed" depending on tone.
- If the customer engaged and the conversation progressed but no booking happened: score 3-4. Sentiment based on customer's actual tone.
- Only score 5 when an appointment was booked AND the customer sounded positive.

FIELDS:
- "quality_score": integer 1-5 only (NEVER outside 1-5)
- "sentiment": "positive" | "neutral" | "negative" | "annoyed" — measure the CUSTOMER's tone, not the agent's
- "appointment_booked": true or false (true only if a specific date/time was confirmed)
- "failure_reason": 1 sentence in English on why the call didn't convert; if it did, write "N/A — appointment booked". For no-reply calls, write e.g. "Customer did not respond after greeting — likely hung up or wrong number"
- "what_went_well": 1-2 sentences in English on what the agent did right. If nothing meaningful happened (no reply), say "Agent delivered greeting clearly but had no chance to engage the customer"
- "what_went_wrong": 1-2 sentences on what the agent could improve. For no-reply calls, say "No opportunity to engage — call ended before any customer interaction"
- "summary": 1-2 sentence summary referencing what specifically happened in THIS transcript
- "insights": 1 coaching insight in English for next time
- "prompt_improvement_suggestion": 1 specific, actionable instruction to add to the AI system prompt to improve future calls of this kind

The transcript may be in any language (Telugu, Hindi, English, etc.); ALWAYS write your analysis fields in English regardless of the transcript language. Reference specific things from THIS transcript — never write generic filler.

Return ONLY valid JSON. No markdown, no explanation. Keep each string under 240 chars.`

func (s *Service) analyzeCall(ctx context.Context, history []llm.ChatMessage) (*analysis, error) {
	transcript := formatTranscript(history)
	userMsg := llm.ChatMessage{Role: "user", Text: "Analyze this call transcript:\n\n" + transcript}

	// 1500 tokens is enough for the 8-key JSON object including 200-char
	// strings each. The previous 512 cap truncated mid-key, causing every
	// post-call analysis to fail JSON parsing → all reviews saved with
	// quality_score=0, sentiment="neutral" defaults. Issue: empty insight
	// columns in the Call Insights tab.
	raw, err := s.llm.GenerateResponse(ctx, analysisSystemPrompt, []llm.ChatMessage{userMsg}, 1500)
	if err != nil {
		return nil, err
	}

	// Strip markdown fences if Gemini wraps in ```json ... ```
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = raw[strings.Index(raw, "\n")+1:]
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}

	var a analysis
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, fmt.Errorf("analysis JSON parse: %w (raw: %s)", err, raw[:min(len(raw), 200)])
	}
	// Clamp quality_score to the 0-5 scale the UI expects.
	if a.QualityScore < 0 {
		a.QualityScore = 0
	}
	if a.QualityScore > 5 {
		a.QualityScore = 5
	}
	if a.Sentiment == "" {
		a.Sentiment = "neutral"
	}
	return &a, nil
}

// ── WA appointment confirmation ───────────────────────────────────────────────

func (s *Service) sendAppointmentConfirmation(ctx context.Context, orgID int64, phone, agentName string) {
	channels, err := s.database.GetWAChannelConfigsByOrg(orgID)
	if err != nil || len(channels) == 0 {
		return
	}
	ch := channels[0]
	cfg := wa.ChannelConfig{
		Provider:    ch.Provider,
		PhoneNumber: ch.PhoneNumber,
		APIKey:      ch.APIKey,
		AppID:       ch.AppID,
	}
	msg := fmt.Sprintf("Hi! Your appointment has been confirmed. Our representative %s will be in touch shortly.", agentName)
	if err := wa.SendText(ctx, cfg, phone, msg); err != nil {
		s.log.Warn("recording: WA appointment confirmation failed", zap.Error(err))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// historyToTranscript builds the persisted transcript turns in the shape the
// frontend reads and Python's recording_service produces:
//
//	[{"role":"AI","text":"..."}, {"role":"User","text":"..."}]
//
// Role mapping follows the Python code exactly (recording_service.py:38):
//   - internal "model" → "AI"   (agent bubble in TranscriptModal)
//   - everything else  → "User" (customer bubble)
//
// Empty-text turns are skipped — matching Python's `if text:` guard — so a
// row is never saved for a "connected but nothing said" call.
//
// Returns (json_string, turn_count). The caller checks turn_count to decide
// whether to persist (Python: `if transcript_turns: save_call_transcript(...)`).
func historyToTranscript(history []llm.ChatMessage) (string, int) {
	type persistedTurn struct {
		Role string `json:"role"`
		Text string `json:"text"`
	}
	out := make([]persistedTurn, 0, len(history))
	for _, m := range history {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		role := "User"
		if m.Role == "model" {
			role = "AI"
		}
		out = append(out, persistedTurn{Role: role, Text: text})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]", 0
	}
	return string(b), len(out)
}

func formatTranscript(history []llm.ChatMessage) string {
	var sb strings.Builder
	for _, m := range history {
		role := "Agent"
		if m.Role == "user" {
			role = "Customer"
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(m.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

var dncKeywords = []string{"do not call", "don't call", "stop calling", "remove me", "not interested", "blocked"}

func containsDNC(history []llm.ChatMessage) bool {
	for _, m := range history {
		if m.Role != "user" {
			continue
		}
		lower := strings.ToLower(m.Text)
		for _, kw := range dncKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

func sanitize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
