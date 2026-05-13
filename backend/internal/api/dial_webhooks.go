package api

import (
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

func (s *Server) exotelStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	callSid := coalesceStr(r.FormValue("CallSid"), r.FormValue("sid"))
	callStatus := coalesceStr(r.FormValue("Status"), r.FormValue("CallStatus"))
	recordingURL := r.FormValue("RecordingUrl")

	if callSid == "" {
		// Try query params (some Exotel setups put them there)
		callSid = r.URL.Query().Get("lead_id") // fallback using lead_id from URL
		callSid = ""                           // reset — can't infer call_sid this way
		w.WriteHeader(http.StatusOK)
		return
	}

	status := mapExotelStatus(callStatus)
	if err := s.db.UpdateCallLogStatus(callSid, status); err != nil {
		s.logger.Warn("exotelStatus: UpdateCallLogStatus",
			zap.String("call_sid", callSid), zap.Error(err))
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
		}
	}

	w.WriteHeader(http.StatusOK)
}

// ── GET|POST /exotel/recording-ready ─────────────────────────────────────────
// Exotel calls this when a recording is ready for download.

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
// Generic CRM push webhook: challenge handshake or new lead notification.

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
	// Build authenticated URL for Exotel recordings
	parsedURL, err := url.Parse(recordingURL)
	if err != nil {
		return fmt.Errorf("invalid recording URL: %w", err)
	}
	if parsedURL.User == nil && s.cfg.ExotelAPIKey != "" {
		parsedURL.User = url.UserPassword(s.cfg.ExotelAPIKey, s.cfg.ExotelAPIToken)
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
	destPath := filepath.Join(s.cfg.RecordingsDir, filename)

	_ = os.MkdirAll(s.cfg.RecordingsDir, 0755)
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	localURL := fmt.Sprintf("%s/api/recordings/%s", s.cfg.PublicServerURL, filename)
	if err := s.db.UpdateCallLogRecordingURL(callSid, localURL); err != nil {
		s.logger.Warn("downloadRecording: UpdateCallLogRecordingURL", zap.Error(err))
	}

	s.logger.Info("recording saved",
		zap.String("call_sid", callSid),
		zap.String("file", filename))
	return nil
}

// ── ensure dial package is used ───────────────────────────────────────────────
var _ = dial.ErrDND
