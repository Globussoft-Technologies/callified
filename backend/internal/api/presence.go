package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/wshandler"
)

// Ensure the wshandler package is imported so we can read UserEmail from active sessions.
var _ wshandler.ActiveSession

const (
	presenceOfflineTimeout = 60 * time.Second
	presenceChannelFmt     = "presence:%d"
)

// updateAgentPresence transitions an agent's status and keeps cumulative idle
// time accurate. When the agent leaves idle we add the elapsed idle interval to
// total_idle_time_s; when they enter idle we record the start timestamp.
func (s *Server) updateAgentPresence(userID int64, status string, onCallSince, breakSince *time.Time) error {
	now := time.Now().UTC()
	current, err := s.db.GetAgentPresence(userID)
	if err != nil {
		return err
	}

	var idleSince *time.Time
	var idleDelta int64
	if status == db.PresenceIdle {
		if current != nil && current.Status == db.PresenceIdle && current.IdleSince != "" {
			if t, pErr := time.Parse(time.RFC3339, current.IdleSince); pErr == nil {
				idleSince = &t
			}
		}
		if idleSince == nil {
			idleSince = &now
		}
	} else {
		if current != nil && current.Status == db.PresenceIdle && current.IdleSince != "" {
			if t, pErr := time.Parse(time.RFC3339, current.IdleSince); pErr == nil {
				idleDelta = int64(now.Sub(t).Seconds())
			}
		}
	}

	return s.db.UpsertAgentPresence(userID, status, onCallSince, breakSince, idleSince, idleDelta)
}

// heartbeatRequest is sent by agents to keep their live status up to date.
type heartbeatRequest struct {
	Status string `json:"status"` // idle | break | on_call
}

// POST /api/presence/heartbeat
// @Summary     Agent presence heartbeat
// @Description Agents ping this endpoint periodically to report their current status.
// @Tags        presence
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{status=string}  false  "status: idle | break | on_call"
// @Success     200  {object}  BoolResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/presence/heartbeat [post]
func (s *Server) presenceHeartbeat(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	u, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || u == nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}

	var body heartbeatRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	status := body.Status
	if status == "" {
		status = db.PresenceIdle
	}
	if status != db.PresenceIdle && status != db.PresenceBreak && status != db.PresenceOnCall {
		status = db.PresenceIdle
	}

	var onCallSince, breakSince *time.Time
	now := time.Now().UTC()
	if status == db.PresenceOnCall {
		onCallSince = &now
	}
	if status == db.PresenceBreak {
		breakSince = &now
	}

	if err := s.updateAgentPresence(u.ID, status, onCallSince, breakSince); err != nil {
		s.logger.Sugar().Errorw("presenceHeartbeat", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.broadcastPresence(u.OrgID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/presence/break
// @Summary     Set agent on break
// @Description Convenience endpoint for the agent dashboard to mark themselves on break.
// @Tags        presence
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  BoolResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/presence/break [post]
func (s *Server) presenceBreak(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	u, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || u == nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	now := time.Now().UTC()
	if err := s.updateAgentPresence(u.ID, db.PresenceBreak, nil, &now); err != nil {
		s.logger.Sugar().Errorw("presenceBreak", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.broadcastPresence(u.OrgID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/presence/idle
// @Summary     Clear break / set agent idle
// @Description Marks the current agent as idle (available).
// @Tags        presence
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  BoolResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/presence/idle [post]
func (s *Server) presenceIdle(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	u, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || u == nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	if err := s.updateAgentPresence(u.ID, db.PresenceIdle, nil, nil); err != nil {
		s.logger.Sugar().Errorw("presenceIdle", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.broadcastPresence(u.OrgID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/presence
// @Summary     List agent presence
// @Description Returns live status for every agent in the org. Admin only.
// @Tags        presence
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.AgentPresenceRow
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/presence [get]
func (s *Server) listPresence(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)

	// Mark stale heartbeats offline before returning.
	_ = s.db.MarkAgentsOffline(time.Now().UTC().Add(-presenceOfflineTimeout))

	rows, err := s.db.GetAgentPresenceByOrg(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listPresence", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Cross-check active websocket sessions (browser / sim web calls) and mark
	// matching users as on_call. Scope to the caller's org so one tenant does
	// not see another tenant's on-call agents.
	onCallEmails := map[string]bool{}
	if s.wsHandler != nil {
		for _, sess := range s.wsHandler.ActiveSessions() {
			if sess.UserEmail != "" && sess.OrgID == ac.OrgID {
				onCallEmails[sess.UserEmail] = true
			}
		}
	}

	for i := range rows {
		if onCallEmails[rows[i].Email] {
			rows[i].Status = db.PresenceOnCall
		}
	}

	writeJSON(w, http.StatusOK, emptyJSON(rows))
}

// GET /api/sse/presence
// @Summary     Presence SSE stream
// @Description Server-sent event stream of agent presence changes for the admin dashboard.
// @Tags        sse
// @Produce     text/event-stream
// @Security    BearerAuth
// @Param       ticket  query  string  false  "Short-lived SSE ticket"
// @Success     200  {string}  string  "data: {...}\n\n"
// @Router      /api/sse/presence [get]
func (s *Server) presenceSSE(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	if !s.isAdminLike(ac.Email) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	channel := fmt.Sprintf(presenceChannelFmt, ac.OrgID)
	msgs := s.store.Subscribe(ctx, channel)

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) broadcastPresence(orgID int64) {
	if s.store == nil {
		return
	}
	_ = s.db.MarkAgentsOffline(time.Now().UTC().Add(-presenceOfflineTimeout))
	rows, err := s.db.GetAgentPresenceByOrg(orgID)
	if err != nil {
		return
	}
	b, _ := json.Marshal(rows)
	s.store.Publish(context.Background(), fmt.Sprintf(presenceChannelFmt, orgID), string(b))
}

// isAdminLike returns true for Admin / SuperAdmin / super_admin users.
func (s *Server) isAdminLike(email string) bool {
	if s.isSuperAdmin(email) {
		return true
	}
	u, err := s.db.GetUserByEmail(email)
	if err != nil || u == nil {
		return false
	}
	return u.Role == db.RoleAdmin || u.Role == "SuperAdmin"
}

