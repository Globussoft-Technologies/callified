package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/notifications ───────────────────────────────────────────────────

// @Summary     List notifications
// @Description Returns the authenticated user's notifications, newest first.
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Param       limit  query  int  false  "Maximum notifications to return"  default(50)
// @Success     200  {array}   db.Notification
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/notifications [get]
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	limit := int64(50)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt64(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	notifications, err := s.db.GetNotificationsForUser(user.ID, limit)
	if err != nil {
		s.logger.Sugar().Errorw("listNotifications", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(notifications))
}

// parseInt64 is a small helper for query-string ints.
func parseInt64(s string) (int64, error) {
	return json.Number(s).Int64()
}

// ── GET /api/notifications/unread-count ──────────────────────────────────────

// @Summary     Unread notification count
// @Description Returns the number of unread notifications for the current user.
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object{count=int64}
// @Router      /api/notifications/unread-count [get]
func (s *Server) unreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	count, err := s.db.GetUnreadNotificationCount(user.ID)
	if err != nil {
		s.logger.Sugar().Errorw("unreadNotificationCount", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}

// ── PUT /api/notifications/{id}/read ─────────────────────────────────────────

// @Summary     Mark notification read
// @Description Marks a single notification as read.
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  int64  true  "Notification ID"
// @Success     200  {object}  BoolResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/notifications/{id}/read [put]
func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if err := s.db.MarkNotificationRead(id, user.ID); err != nil {
		s.logger.Sugar().Errorw("markNotificationRead", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// ── PUT /api/notifications/read-all ──────────────────────────────────────────

// @Summary     Mark all notifications read
// @Description Marks every notification for the current user as read.
// @Tags        notifications
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  BoolResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/notifications/read-all [put]
func (s *Server) markAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if err := s.db.MarkAllNotificationsRead(user.ID); err != nil {
		s.logger.Sugar().Errorw("markAllNotificationsRead", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// publishNotification sends a notification to a user's SSE channel and persists
// it in the database. Used by campaign assignment and future event sources.
func (s *Server) publishNotification(ctx context.Context, userID int64, nType, title, body string, payload map[string]any) {
	payloadJSON := ""
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			payloadJSON = string(b)
		}
	}
	nid, err := s.db.CreateNotification(userID, nType, title, body, payloadJSON)
	if err != nil {
		s.logger.Sugar().Warnw("publishNotification: create failed", "user_id", userID, "err", err)
		return
	}
	if s.store == nil {
		return
	}
	n := db.Notification{
		ID:        nid,
		UserID:    userID,
		Type:      nType,
		Title:     title,
		Body:      body,
		Payload:   payloadJSON,
		IsRead:    false,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if b, err := json.Marshal(n); err == nil {
		s.store.Publish(ctx, "user-notifications:"+strconv.FormatInt(userID, 10), string(b))
	}
}
