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

// GET /api/active-calls
// @Summary     List active calls
// @Description Returns all currently active call sessions with stream SIDs and monitor URLs. Requires Admin role.
// @Tags        calls
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object{count=int,active_calls=array}
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Router      /api/active-calls [get]
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
