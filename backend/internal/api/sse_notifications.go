package api

import (
	"fmt"
	"net/http"
	"time"
)

// GET /api/sse/notifications
// @Summary     Notification stream (SSE)
// @Description Server-sent event stream for in-app notifications. Authenticate via ?ticket=<sse-ticket> or Authorization header.
// @Tags        sse
// @Produce     text/event-stream
// @Security    BearerAuth
// @Param       ticket  query  string  false  "Short-lived SSE ticket"
// @Success     200  {string}  string  "event: notification\ndata: {...}\n\n"
// @Router      /api/sse/notifications [get]
func (s *Server) notificationEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ac := getAuth(r)
	user, err := s.db.GetUserByEmail(ac.Email)
	if err != nil || user == nil {
		fmt.Fprint(w, "event: error\ndata: unauthorized\n\n")
		flusher.Flush()
		return
	}

	// Initial ack.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	channel := fmt.Sprintf("user-notifications:%d", user.ID)
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
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
