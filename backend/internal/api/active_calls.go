package api

import (
	"net/http"

	"github.com/globussoft/callified-backend/internal/wshandler"
)

// activeCallLister is the small interface the API server needs from
// wshandler.Handler — kept as an interface so api doesn't take a hard
// dependency on the gorilla/websocket transitive types in tests.
type activeCallLister interface {
	ActiveSessions() []wshandler.ActiveSession
}

// SetWSHandler wires the WebSocket handler so /api/active-calls can list
// live sessions. Called from main.go after both are constructed.
func (s *Server) SetWSHandler(h activeCallLister) {
	s.wsHandler = h
}

// activeCalls returns a JSON list of every currently-active call session.
// Admin-gated (PII: lead names + phone numbers). Useful for ops dashboards
// and as a way to grab a live stream_sid for /ws/monitor/{sid} without
// tailing backend logs.
//
//	GET /api/active-calls
//	→ 200 {"count": 2, "active_calls": [{stream_sid, monitor_url, ...}]}
func (s *Server) activeCalls(w http.ResponseWriter, r *http.Request) {
	if s.wsHandler == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"count":        0,
			"active_calls": []any{},
		})
		return
	}
	sessions := s.wsHandler.ActiveSessions()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":        len(sessions),
		"active_calls": sessions,
	})
}
