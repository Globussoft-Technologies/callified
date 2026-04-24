package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

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
	if campaignID == "all" {
		for _, past := range s.store.RecentAllCampaignEvents(ctx, 20) {
			fmt.Fprintf(w, "data: %s\n\n", past)
		}
		flusher.Flush()
	} else if cid, err := strconv.ParseInt(campaignID, 10, 64); err == nil && cid > 0 {
		for _, past := range s.store.RecentCampaignEvents(ctx, cid, 20) {
			fmt.Fprintf(w, "data: %s\n\n", past)
		}
		flusher.Flush()
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
