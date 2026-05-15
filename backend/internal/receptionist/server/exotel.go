package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// exotelVoice handles GET/POST /api/receptionist/exotel/voice — the
// Passthru applet URL configured on the Exotel App. We respond with
// ExoML telling the carrier to <Connect><Stream> to our WebSocket so
// the receptionist's wsphone pipeline receives the audio.
//
// Why both GET and POST: Exotel's Passthru applet defaults to GET but
// can be configured for POST. Accepting both is cheaper than discovering
// the dashboard config is wrong an hour into a demo.
//
// The WS URL is constructed from PUBLIC_SERVER_URL (env). On testgo1
// that's "https://testgo1.callified.ai", so the WS becomes
// "wss://testgo1.callified.ai/api/receptionist/media-stream". Carrier
// query params (CallSid, From, To) are forwarded so the WS handler can
// log them before the formal "start" event lands.
func (s *Server) exotelVoice(w http.ResponseWriter, r *http.Request) {
	publicURL := os.Getenv("PUBLIC_SERVER_URL")
	if publicURL == "" {
		// Without PUBLIC_SERVER_URL we can't tell Exotel where to connect.
		// 503 is the correct signal — server-side misconfiguration the
		// caller can't help us with.
		writeErr(w, 503, "PUBLIC_SERVER_URL not configured")
		return
	}

	// Carrier may send these as form-POST or as query params depending on
	// dashboard config; merge both so we don't have to know which.
	_ = r.ParseForm()
	q := r.URL.Query()
	getParam := func(name string) string {
		if v := r.FormValue(name); v != "" {
			return v
		}
		return q.Get(name)
	}

	callSid := getParam("CallSid")
	from := getParam("From")
	to := getParam("To")

	wsBase := strings.Replace(publicURL, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	wsURL := fmt.Sprintf("%s/api/receptionist/media-stream?from=%s&to=%s&call_sid=%s",
		wsBase,
		url.QueryEscape(from),
		url.QueryEscape(to),
		url.QueryEscape(callSid),
	)

	// ExoML uses Twilio-compatible XML. <Connect><Stream> opens a duplex
	// audio stream to wsURL. Exotel's <Parameter> children are not
	// currently used by our handler (we read the same fields from the
	// "start" frame's payload), but we include them so the wsphone log
	// has identity info even if the start-event payload is malformed.
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Connect>
    <Stream url="%s">
      <Parameter name="from" value="%s"/>
      <Parameter name="to" value="%s"/>
      <Parameter name="call_sid" value="%s"/>
    </Stream>
  </Connect>
</Response>`,
		escapeXML(wsURL),
		escapeXML(from),
		escapeXML(to),
		escapeXML(callSid),
	)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// exotelStatus is the call-status webhook (initiated, ringing, answered,
// completed). Stage 1 acks all events with 200 — wiring real handling
// (call_log persistence, retry-on-failure) is a later concern.
func (s *Server) exotelStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
