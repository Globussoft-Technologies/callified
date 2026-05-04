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
	s.logger.Info("exotelXML: Exotel fetched ExoML",
		zap.String("lead_id", q.Get("lead_id")),
		zap.String("campaign_id", q.Get("campaign_id")),
		zap.String("phone", q.Get("phone")),
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("user_agent", r.Header.Get("User-Agent")),
	)
	wsURL := strings.Replace(s.cfg.PublicServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = fmt.Sprintf("%s/media-stream?name=%s&interest=%s&phone=%s&lead_id=%s&campaign_id=%s&org_id=%s",
		wsURL,
		url.QueryEscape(q.Get("name")),
		url.QueryEscape(q.Get("interest")),
		url.QueryEscape(q.Get("phone")),
		url.QueryEscape(q.Get("lead_id")),
		url.QueryEscape(q.Get("campaign_id")),
		url.QueryEscape(q.Get("org_id")),
	)
	s.logger.Info("exotelXML: serving ExoML with stream URL", zap.String("ws_url", wsURL))
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
		}
	}

	w.WriteHeader(http.StatusOK)
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
