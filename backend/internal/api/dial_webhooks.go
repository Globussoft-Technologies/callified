package api

import (
	"context"
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

	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
)

// ── GET /webhook/twilio ───────────────────────────────────────────────────────
// Twilio calls this URL when a call connects. We return TwiML that opens a
// media stream back to this server.

// GET /webhook/twilio
// @Summary     Twilio TwiML hook
// @Description Returns TwiML connecting the call to the media-stream WebSocket. Called by Twilio.
// @Tags        webhooks
// @Produce     text/xml
// @Param       lead_id      query  int64  false  "Lead ID"
// @Param       campaign_id  query  int64  false  "Campaign ID"
// @Param       org_id       query  int64  false  "Org ID"
// @Success     200  {string}  string  "TwiML response"
// @Router      /webhook/twilio [get]
func (s *Server) twilioTwiML(w http.ResponseWriter, r *http.Request) {
	leadID := r.URL.Query().Get("lead_id")
	campaignID := r.URL.Query().Get("campaign_id")
	orgID := r.URL.Query().Get("org_id")

	wsURL := strings.Replace(s.cfg.PublicServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf("%s/media-stream?lead_id=%s&campaign_id=%s&org_id=%s", wsURL, leadID, campaignID, orgID)

	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Connect>
    <Stream url="%s">
      <Parameter name="lead_id" value="%s"/>
      <Parameter name="campaign_id" value="%s"/>
      <Parameter name="org_id" value="%s"/>
    </Stream>
  </Connect>
</Response>`, wsURL, leadID, campaignID, orgID)

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(twiml))
}

// ── GET/POST /webhook/exotel ─────────────────────────────────────────────────
//
// Returns dynamic XML telling the carrier to <Connect><Stream> the call to
// our /media-stream WebSocket. Matches Python's dynamic_webhook (Backup_Callified
// webhook_routes.py:48-56) which served the same XML for both Twilio and Exotel.
//
// Why this exists: the ExoML app on the Exotel dashboard (the one referenced
// by EXOTEL_APP_ID) typically has a Passthru applet that fetches XML from
// {PUBLIC_SERVER_URL}/webhook/exotel. When Python ran, Python served this and
// the call connected. After switching to Go, this URL was 404'ing — Exotel
// got no instructions and the call never reached our WebSocket. With this
// handler in place the existing dashboard config works again, no Exotel-side
// change required.
//
// Carrier-passed query params (name / interest / phone) are forwarded into
// the WebSocket URL so the WS handler can hydrate the session immediately
// without waiting for Redis lookup.

// GET /webhook/exotel
// @Summary     Exotel ExoML hook
// @Description Returns ExoML connecting the call to the media-stream WebSocket. Called by Exotel's Passthru applet.
// @Tags        webhooks
// @Produce     application/xml
// @Param       CallSid   query  string  false  "Exotel call SID"
// @Param       CallFrom  query  string  false  "Caller phone number"
// @Success     200  {string}  string  "ExoML response"
// @Router      /webhook/exotel [get]
func (s *Server) exotelXML(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Look up the campaign-resolved voice settings + lead context that
	// dial.Initiator stashed in Redis at the moment we asked Exotel to dial.
	// Doing the lookup here (one hop earlier than the WS-handler's
	// handleStartEvent) lets us bake everything into the stream URL so the
	// WS opens with the same full context a Sim Web Call does — no race
	// between STT init and Redis hydration. The WS handler's existing
	// hydration block stays as a safety net if this lookup misses.
	callSid := firstNonEmptyStr(q.Get("CallSid"), q.Get("call_sid"))
	phone := firstNonEmptyStr(q.Get("From"), q.Get("CallFrom"), q.Get("phone"))

	var pending rstore.PendingCallInfo
	var hitKey string
	if callSid != "" {
		if p, ok := s.store.GetPendingCall(r.Context(), callSid); ok {
			pending = p
			hitKey = "call_sid"
		}
	}
	if pending.LeadID == 0 && phone != "" {
		if p, ok := s.store.GetPendingCall(r.Context(), "phone:"+phone); ok {
			pending = p
			hitKey = "phone"
		}
	}
	if pending.LeadID == 0 {
		if p, ok := s.store.GetPendingCall(r.Context(), "latest"); ok {
			pending = p
			hitKey = "latest"
		}
	}

	// Fall back to whatever the Passthru applet happened to forward when
	// Redis didn't have a record. (Empty values just produce empty query
	// params — the WS handler's handleStartEvent still backfills from
	// Redis after the start frame arrives, so this is best-effort.)
	pickInt64 := func(redisVal int64, qKey string) int64 {
		if redisVal != 0 {
			return redisVal
		}
		var n int64
		fmt.Sscanf(q.Get(qKey), "%d", &n)
		return n
	}
	pickStr := func(redisVal string, qKey string) string {
		if redisVal != "" {
			return redisVal
		}
		return q.Get(qKey)
	}

	name := pickStr(pending.Name, "name")
	interest := pickStr(pending.Interest, "interest")
	phoneOut := pickStr(pending.Phone, "phone")
	leadID := pickInt64(pending.LeadID, "lead_id")
	campaignID := pickInt64(pending.CampaignID, "campaign_id")
	orgID := pickInt64(pending.OrgID, "org_id")
	ttsProvider := pending.TTSProvider
	ttsVoiceID := pending.TTSVoiceID
	ttsLanguage := pending.TTSLanguage

	wsURL := strings.Replace(s.cfg.PublicServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf(
		"%s/media-stream?name=%s&interest=%s&phone=%s&lead_id=%d&campaign_id=%d&org_id=%d&tts_provider=%s&voice=%s&tts_language=%s",
		wsURL,
		url.QueryEscape(name),
		url.QueryEscape(interest),
		url.QueryEscape(phoneOut),
		leadID,
		campaignID,
		orgID,
		url.QueryEscape(ttsProvider),
		url.QueryEscape(ttsVoiceID),
		url.QueryEscape(ttsLanguage),
	)

	s.logger.Info("exotelXML: serving ExoML",
		zap.String("call_sid", callSid),
		zap.String("redis_hit", hitKey),
		zap.Int64("lead_id", leadID),
		zap.Int64("campaign_id", campaignID),
		zap.Int64("org_id", orgID),
		zap.String("tts_provider", ttsProvider),
		zap.String("tts_voice_id", ttsVoiceID),
		zap.String("tts_language", ttsLanguage),
		zap.String("ws_url", wsURL),
		zap.String("remote_addr", r.RemoteAddr),
	)

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Connect>
    <Stream url="%s"/>
  </Connect>
</Response>`, wsURL)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml))
}

// firstNonEmptyStr returns the first non-empty value in vals, or "" if none.
// Local helper because wshandler.firstNonEmpty isn't exported.
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── POST /webhook/twilio/status ───────────────────────────────────────────────
// Twilio posts call status updates here (initiated, ringing, answered, completed).

// POST /webhook/twilio/status
// @Summary     Twilio call status hook
// @Description Receives call status updates from Twilio (initiated, ringing, completed, failed, etc.).
// @Tags        webhooks
// @Accept      application/x-www-form-urlencoded
// @Success     200  "OK"
// @Router      /webhook/twilio/status [post]
func (s *Server) twilioStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK) // always 200 for Twilio
		return
	}

	callSid := r.FormValue("CallSid")
	callStatus := r.FormValue("CallStatus")

	if callSid == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Map Twilio status → our internal status
	status := mapTwilioStatus(callStatus)
	if err := s.db.UpdateCallLogStatus(callSid, status); err != nil {
		s.logger.Warn("twilioStatus: UpdateCallLogStatus",
			zap.String("call_sid", callSid), zap.Error(err))
	}

	// On completion, fire webhook event
	if callStatus == "completed" || callStatus == "failed" || callStatus == "no-answer" || callStatus == "busy" {
		cl, _ := s.db.GetCallLogByCallSid(callSid)
		if cl != nil {
			s.dispatcher.Dispatch(r.Context(), cl.OrgID, "call.completed", map[string]any{
				"call_sid":   callSid,
				"status":     callStatus,
				"lead_id":    cl.LeadID,
				"campaign_id": cl.CampaignID,
			})
			s.enqueueRetryIfFailed(cl.LeadID, cl.CampaignID, cl.OrgID, status)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// enqueueRetryIfFailed inserts a row into call_retries when a call ends in a
// transient-failure status that's worth re-attempting. The retry_worker
// drains pending rows every 2 minutes and re-dials. Issue #77 — the table
// existed and the worker was already running, but no caller ever populated
// the queue, so the Retries tab stayed empty after every failed call.
//
// Skips:
//   - non-failure statuses (completed / connected / answered)
//   - leads that already have an active retry chain in this campaign
//     (avoids duplicate or runaway retry loops)
//
// Defaults: 3 max attempts, first retry 30 minutes after the failure to
// give transient telco issues time to clear. The worker's own backoff
// (30m → 1h → 2h) takes over from there.
func (s *Server) enqueueRetryIfFailed(leadID, campaignID, orgID int64, status string) {
	switch status {
	case "failed", "no-answer", "busy", "no_answer", "cancelled":
		// retryable — fall through
	default:
		return
	}
	if leadID == 0 {
		return
	}
	already, err := s.db.HasPendingOrExhaustedRetry(leadID, campaignID)
	if err != nil {
		s.logger.Warn("enqueueRetryIfFailed: HasPendingOrExhaustedRetry",
			zap.Int64("lead_id", leadID), zap.Error(err))
		return
	}
	if already {
		return
	}
	nextAttempt := time.Now().Add(30 * time.Minute)
	if _, err := s.db.CreateRetry(leadID, campaignID, orgID, 3, nextAttempt); err != nil {
		s.logger.Warn("enqueueRetryIfFailed: CreateRetry",
			zap.Int64("lead_id", leadID), zap.String("status", status), zap.Error(err))
		return
	}
	s.logger.Info("enqueueRetryIfFailed: queued",
		zap.Int64("lead_id", leadID), zap.Int64("campaign_id", campaignID),
		zap.String("reason", status))
}

// ── POST /webhook/exotel/status ───────────────────────────────────────────────
// Exotel posts call status updates here.

// POST /webhook/exotel/status
// @Summary     Exotel call status hook
// @Description Receives call status updates from Exotel (completed, failed, no-answer, etc.).
// @Tags        webhooks
// @Accept      application/x-www-form-urlencoded
// @Success     200  "OK"
// @Router      /webhook/exotel/status [post]
func (s *Server) exotelStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	callSid := coalesceStr(r.FormValue("CallSid"), r.FormValue("sid"))
	callStatus := coalesceStr(r.FormValue("Status"), r.FormValue("CallStatus"))
	recordingURL := r.FormValue("RecordingUrl")
	var callDurationS float64
	if d := coalesceStr(r.FormValue("CallDuration"), r.FormValue("Duration")); d != "" {
		fmt.Sscanf(d, "%f", &callDurationS)
	}

	// Log every status event so we can verify what Exotel actually sends.
	s.logger.Info("exotelStatus: received",
		zap.String("call_sid", callSid),
		zap.String("raw_status", callStatus),
		zap.String("all_form", r.Form.Encode()),
	)

	if callSid == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	status := mapExotelStatus(callStatus)

	// Customer answered — ungate agent audio in bridge sessions.
	// Handle both "in-progress" and "answered" since Exotel may send either.
	rawLower := strings.ToLower(callStatus)
	if status == "in-progress" || rawLower == "answered" || rawLower == "in-progress" || rawLower == "inprogress" {
		s.store.MarkBridgeAnswered(r.Context(), callSid)
	}

	if err := s.db.UpdateCallLogStatus(callSid, status); err != nil {
		s.logger.Warn("exotelStatus: UpdateCallLogStatus",
			zap.String("call_sid", callSid), zap.Error(err))
	}
	if callDurationS > 0 {
		_ = s.db.UpdateHumanCallTranscriptDuration(callSid, callDurationS)
	}

	if recordingURL != "" {
		// Schedule async recording download
		go s.fetchAndSaveRecording(callSid, recordingURL)
	}

	// Emit live-feed event + fire webhook on terminal states. The
	// EmitCampaignEvent call is what populates the "Live Campaign Activity"
	// panel on the campaign detail page — matches Python's emit_campaign_event
	// calls in webhook_routes.py:101/108.
	if status == "completed" || status == "failed" || status == "no-answer" || status == "busy" {
		cl, _ := s.db.GetCallLogByCallSid(callSid)
		if cl != nil {
			// Pull lead details for a friendly "Name (phone)" label.
			var leadName string
			if lead, err := s.db.GetLeadByID(cl.LeadID); err == nil && lead != nil {
				leadName = strings.TrimSpace(lead.FirstName + " " + lead.LastName)
			}
			s.store.EmitCampaignEvent(r.Context(), cl.CampaignID, leadName, cl.Phone, status,
				fmt.Sprintf("Exotel: %s", callStatus))
			s.dispatcher.Dispatch(r.Context(), cl.OrgID, "call.completed", map[string]any{
				"call_sid":    callSid,
				"status":      status,
				"lead_id":     cl.LeadID,
				"campaign_id": cl.CampaignID,
			})
			s.enqueueRetryIfFailed(cl.LeadID, cl.CampaignID, cl.OrgID, status)

			// For human (agent-bridged) calls, Exotel often omits RecordingUrl from
			// the StatusCallback. Re-trigger recording polling so a server restart
			// between dial-time and recording-ready doesn't leave the transcript empty.
			if status == "completed" && recordingURL == "" && cl.Provider == "exotel-human" {
				if creds, cerr := s.db.GetCampaignExotelCreds(cl.CampaignID); cerr == nil && creds.IsSet() {
					go s.pollHumanCallRecording(callSid,
						creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID,
						cl.LeadID, cl.CampaignID, cl.OrgID, 30*time.Second)
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// ── GET|POST /exotel/recording-ready ─────────────────────────────────────────
// Exotel calls this when a recording is ready for download.

// GET /exotel/recording-ready
// @Summary     Exotel recording ready hook
// @Description Called by Exotel when a call recording is available for download.
// @Tags        webhooks
// @Success     200  "OK"
// @Router      /exotel/recording-ready [get]
func (s *Server) exotelRecordingReady(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	callSid := coalesceStr(r.FormValue("CallSid"), r.FormValue("sid"))
	recordingURL := coalesceStr(r.FormValue("RecordingUrl"), r.FormValue("recording_url"))

	if callSid != "" && recordingURL != "" {
		go s.fetchAndSaveRecording(callSid, recordingURL)
	}

	w.WriteHeader(http.StatusOK)
}

// ── POST /crm-webhook ─────────────────────────────────────────────────────────

// GET /crm-webhook
// @Summary     CRM push webhook
// @Description Receives CRM push notifications (new lead events) or responds to hub challenge verification.
// @Tags        webhooks
// @Param       hub.challenge  query  string  false  "HubSpot challenge string"
// @Success     200  "OK or challenge echo"
// @Router      /crm-webhook [get]
func (s *Server) crmWebhook(w http.ResponseWriter, r *http.Request) {
	// HubSpot-style GET challenge verification
	if r.Method == http.MethodGet {
		challenge := r.URL.Query().Get("hub.challenge")
		if challenge != "" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(challenge))
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.logger.Info("crm_webhook: received payload",
		zap.Int("bytes", len(body)),
		zap.String("content_type", r.Header.Get("Content-Type")))

	// Attempt to parse as a new lead
	// The CRM provider may vary; we do best-effort parsing.
	// Full per-provider parsing happens in the CRM poller; this is for real-time pushes.
	w.WriteHeader(http.StatusOK)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mapTwilioStatus(s string) string {
	switch s {
	case "initiated", "queued":
		return "initiated"
	case "ringing":
		return "ringing"
	case "in-progress":
		return "in-progress"
	case "completed":
		return "completed"
	case "busy":
		return "busy"
	case "no-answer":
		return "no-answer"
	case "failed", "canceled":
		return "failed"
	default:
		return s
	}
}

func mapExotelStatus(s string) string {
	switch strings.ToLower(s) {
	case "in-progress", "inprogress":
		return "in-progress"
	case "completed", "complete":
		return "completed"
	case "failed", "fail":
		return "failed"
	case "busy":
		return "busy"
	case "no-answer", "noanswer":
		return "no-answer"
	default:
		return s
	}
}

// fetchAndSaveRecording downloads a recording URL with up to 6 retries (10s backoff)
// and saves it to the recordings directory, then updates the DB.
func (s *Server) fetchAndSaveRecording(callSid, recordingURL string) {
	const maxRetries = 6
	const retryDelay = 10 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.downloadRecording(callSid, recordingURL)
		if err == nil {
			return
		}
		s.logger.Warn("fetchAndSaveRecording: attempt failed",
			zap.Int("attempt", attempt), zap.String("call_sid", callSid), zap.Error(err))
		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}
	s.logger.Error("fetchAndSaveRecording: exhausted retries", zap.String("call_sid", callSid))
}

func (s *Server) downloadRecording(callSid, recordingURL string) error {
	// Build authenticated URL for Exotel recordings.
	// Prefer per-callSid creds stored at dial time (for per-campaign Exotel accounts),
	// fall back to the global config credentials.
	parsedURL, err := url.Parse(recordingURL)
	if err != nil {
		return fmt.Errorf("invalid recording URL: %w", err)
	}
	if parsedURL.User == nil {
		apiKey, apiToken := s.cfg.ExotelAPIKey, s.cfg.ExotelAPIToken
		if raw, ok := s.store.GetRaw(context.Background(), "exotel_creds:"+callSid); ok {
			var m map[string]string
			if json.Unmarshal([]byte(raw), &m) == nil {
				if k, t := m["api_key"], m["api_token"]; k != "" && t != "" {
					apiKey, apiToken = k, t
				}
			}
		}
		if apiKey != "" {
			parsedURL.User = url.UserPassword(apiKey, apiToken)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(parsedURL.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d downloading recording", resp.StatusCode)
	}

	ext := ".mp3"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "wav") {
		ext = ".wav"
	}

	filename := fmt.Sprintf("recording_%s%s", callSid, ext)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var localURL string
	if s.s3 != nil {
		s3Key := "recordings/" + filename
		publicURL, err := s.s3.UploadPublic(context.Background(), s3Key, data)
		if err != nil {
			s.logger.Warn("downloadRecording: S3 upload failed", zap.Error(err))
			// Fall through to local save.
		} else {
			localURL = publicURL
			s.logger.Info("downloadRecording: uploaded to S3", zap.String("url", publicURL))
		}
	}

	if localURL == "" {
		destPath := filepath.Join(s.cfg.RecordingsDir, filename)
		_ = os.MkdirAll(s.cfg.RecordingsDir, 0755)
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		localURL = fmt.Sprintf("/api/recordings/%s", filename)
	}
	if err := s.db.UpdateCallLogRecordingURL(callSid, localURL); err != nil {
		s.logger.Warn("downloadRecording: UpdateCallLogRecordingURL", zap.Error(err))
	}
	// For human calls a call_transcripts stub was pre-created at dial time.
	// Try Redis first (fast path); fall back to DB lookup so a server restart
	// between dial and recording-ready doesn't leave the transcript empty.
	transcriptUpdated := false
	if raw, ok := s.store.GetRaw(context.Background(), "transcript_id:"+callSid); ok {
		var transcriptID int64
		fmt.Sscanf(raw, "%d", &transcriptID)
		if transcriptID > 0 {
			if err := s.db.UpdateCallTranscriptRecording(transcriptID, localURL); err != nil {
				s.logger.Warn("downloadRecording: UpdateCallTranscriptRecording", zap.Error(err))
			} else {
				transcriptUpdated = true
			}
		}
	}
	if !transcriptUpdated {
		// Redis key expired or server restarted — look up the call log to get
		// lead_id + campaign_id and update the human-call stub via DB query.
		if cl, err := s.db.GetCallLogByCallSid(callSid); err == nil && cl != nil && cl.Provider == "exotel-human" {
			if err := s.db.UpdateHumanCallTranscriptRecording(cl.LeadID, cl.CampaignID, callSid, localURL); err != nil {
				s.logger.Warn("downloadRecording: UpdateHumanCallTranscriptRecording fallback", zap.Error(err))
			}
		}
	}

	s.logger.Info("recording saved",
		zap.String("call_sid", callSid),
		zap.String("file", filename))
	return nil
}

// ── GET /webhook/exotel/human-call ───────────────────────────────────────────
// Exotel fetches this URL when the agent picks up on a human call initiated via
// POST /api/campaigns/{id}/human-call/{lead_id}. Returns ExoML that announces
// the customer's name and then dials their number to bridge both parties.

// GET /webhook/exotel/human-call
// @Summary     Human call ExoML hook
// @Description Returns ExoML that announces the customer and bridges the agent to them. Called by Exotel when the agent answers.
// @Tags        webhooks
// @Produce     application/xml
// @Param       customer_phone  query  string  true   "Customer phone number"
// @Param       customer_name   query  string  false  "Customer display name"
// @Success     200  {string}  string  "ExoML response"
// @Router      /webhook/exotel/human-call [get]
func (s *Server) exotelHumanCallXML(w http.ResponseWriter, r *http.Request) {
	customerPhone := r.URL.Query().Get("customer_phone")
	customerName := r.URL.Query().Get("customer_name")
	if customerName == "" {
		customerName = "the customer"
	}
	phone := dial.ExotelPhone(customerPhone)
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Say>Connecting you to %s</Say>
  <Dial>%s</Dial>
</Response>`, customerName, phone)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml))
}

// ── ensure dial package is used ───────────────────────────────────────────────
var _ = dial.ErrDND
