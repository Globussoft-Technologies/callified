package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// eventTsBefore reports whether a campaign-event JSON payload's "ts" field
// is older than the given unix timestamp. Used to drop historical events
// that predate the operator's clearedAt marker on SSE replay. Falls back
// to returning false (i.e. keep the event) if the payload can't be parsed
// — we'd rather over-show than silently swallow events. The history
// payload is always JSON in the Go backend (see EmitCampaignEvent), so
// the parse path is the happy path.
func eventTsBefore(payload string, clearedAtUnix int64) bool {
	var p struct {
		Ts string `json:"ts"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil || p.Ts == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, p.Ts)
	if err != nil {
		return false
	}
	return t.Unix() < clearedAtUnix
}

// GET /api/sse/live-logs  — SSE stream of live log entries from Redis pub/sub
func (s *Server) liveLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Send initial ping
	fmt.Fprint(w, "event: ping\ndata: connected\n\n")
	flusher.Flush()

	// Subscribe to live-logs channel
	ctx := r.Context()
	msgs := s.store.Subscribe(ctx, "live-logs")

	// Heartbeat ticker to keep connection alive through load balancers
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
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// GET /api/campaign-events?campaign_id={id}  — frontend-facing alias
func (s *Server) campaignEventsQuery(w http.ResponseWriter, r *http.Request) {
	campaignID := r.URL.Query().Get("campaign_id")
	if campaignID == "" || campaignID == "0" {
		campaignID = "all"
	}
	s.streamCampaignEvents(w, r, campaignID)
}

// DELETE /api/campaign-events?campaign_id={id} — wipe the Redis history
// behind the System Logs page AND set a per-user "clearedAt" marker so a
// refresh doesn't replay events that landed in the history between Clear
// and reload (which is why the server visibly differed from local —
// dialler traffic kept re-LPUSHing into the firehose). campaign_id is
// optional; omit (or pass 0 / "all") to clear only the global firehose,
// pass a numeric id to ALSO clear that campaign's per-campaign list.
// Returns 204 No Content.
//
// JWT-gated (the route is wrapped in auth() in server.go) so an anonymous
// caller can't blow away an org's history.
func (s *Server) clearCampaignEvents(w http.ResponseWriter, r *http.Request) {
	var campaignID int64
	scope := "all"
	if raw := r.URL.Query().Get("campaign_id"); raw != "" && raw != "all" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			campaignID = n
			scope = strconv.FormatInt(n, 10)
		}
	}
	s.store.ClearCampaignEvents(r.Context(), campaignID)
	// Per-(user, scope) marker so the user's next refresh of the SAME view
	// stays empty, but other views (e.g. clearing /logs doesn't blank the
	// campaign detail panel's Live Activity).
	ac := getAuth(r)
	if ac.Email != "" {
		s.store.SetLogsClearedAt(r.Context(), ac.Email, scope, time.Now())
	}
	s.logger.Sugar().Infow("logs cleared", "email", ac.Email, "scope", scope)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/sse/campaign/{id}/events  — SSE stream for campaign dial progress
func (s *Server) campaignEvents(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("id")
	s.streamCampaignEvents(w, r, campaignID)
}

func (s *Server) streamCampaignEvents(w http.ResponseWriter, r *http.Request, campaignID string) {

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Initial ack as an SSE comment (not a named event) — keeps the stream
	// warm for proxies but doesn't show up on the client as a message.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()

	// Replay recent history first so a freshly-opened page immediately shows
	// the last ~20 events instead of an empty panel waiting for the next
	// dial. Matches Python's live_logs.py:90-94 which pushes the last 20
	// items from the in-memory deque on connect.
	//
	// "all" (firehose / System Logs page) reads the global history; numeric
	// IDs (Campaign Detail page) read their per-campaign history.
	//
	// Optional caller opt-out: /logs passes ?no_replay=1 because it wants
	// refresh to start blank — operators don't want events from before
	// the page loaded. Campaign detail panels omit the flag and keep
	// receiving the replay (so the panel isn't blank until the next dial).
	noReplay := r.URL.Query().Get("no_replay") == "1"

	// Filter against the per-(user, scope) clearedAt marker. scope matches
	// the SSE channel suffix: "all" for the firehose, the campaign ID for
	// a per-campaign panel. Per-scope so clearing /logs doesn't hide past
	// events on the campaign detail page (and vice-versa).
	var clearedAt int64
	ac := getAuth(r)
	if ac.Email != "" {
		clearedAt = s.store.GetLogsClearedAt(ctx, ac.Email, campaignID)
	}
	if !noReplay {
		if campaignID == "all" {
			for _, past := range s.store.RecentAllCampaignEvents(ctx, 20) {
				if clearedAt > 0 && eventTsBefore(past, clearedAt) {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", past)
			}
			flusher.Flush()
		} else if cid, err := strconv.ParseInt(campaignID, 10, 64); err == nil && cid > 0 {
			for _, past := range s.store.RecentCampaignEvents(ctx, cid, 20) {
				if clearedAt > 0 && eventTsBefore(past, clearedAt) {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", past)
			}
			flusher.Flush()
		}
	}

	msgs := s.store.Subscribe(ctx, "campaign:"+campaignID)

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
			// Unnamed SSE event (just `data:`) so the frontend's default
			// `EventSource.onmessage` handler fires. Previously we sent
			// `event: campaign\ndata: …` which requires
			// `addEventListener('campaign', …)` — the frontend doesn't
			// register that listener, so every published event was
			// silently dropped. Matches Python's live_logs.py format.
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
