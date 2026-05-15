package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WaSender session-management endpoints. These proxy to wasenderapi.com
// using the org's stored Personal Access Token (saved as the api_key on
// the wasender row in wa_channel_configs). The token is never sent to
// the browser — all calls go server-side.
//
// Why proxy instead of letting the frontend call WaSender directly:
// (1) keeps the PAT off the wire to the browser, (2) lets us standardize
// the response shape so other providers (Gupshup-Connect, future Wati
// QR flow) plug into the same UI, (3) lets us cache aggressively if QR
// polling burns rate limit.

const wasenderBaseURL = "https://www.wasenderapi.com"
const wasenderHTTPTimeout = 10 * time.Second

// waSessionPAT returns the WaSender Personal Access Token saved on the
// org's WhatsApp channel config. Returns "" with a 4xx written to w when
// the org has no wasender config yet — caller should `return` immediately.
func (s *Server) waSessionPAT(w http.ResponseWriter, r *http.Request) string {
	ac := getAuth(r)
	configs, err := s.db.GetWAChannelConfigsByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return ""
	}
	for _, c := range configs {
		if c.Provider == "wasender" && c.APIKey != "" {
			return c.APIKey
		}
	}
	writeError(w, http.StatusBadRequest, "no wasender config — save your Personal Access Token in WhatsApp Channel Config first")
	return ""
}

// waSenderHTTP is the HTTP client used for outbound calls to WaSender.
// Short timeout — these are control-plane calls, not message sends; if
// WaSender is slow, fail fast so the dashboard UI doesn't hang.
var waSenderHTTP = &http.Client{Timeout: wasenderHTTPTimeout}

// proxyToWaSender performs the outbound call and writes WaSender's
// response straight through to the dashboard. Status code is preserved
// so the frontend can distinguish "not initialized yet" (4xx) from
// "WaSender's API is broken" (5xx).
func (s *Server) proxyToWaSender(w http.ResponseWriter, r *http.Request, method, path, pat string, body io.Reader) {
	url := wasenderBaseURL + path
	req, err := http.NewRequestWithContext(r.Context(), method, url, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "build request: "+err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := waSenderHTTP.Do(req)
	if err != nil {
		s.logger.Warn("wa session proxy failed", zap.String("url", url), zap.Error(err))
		writeError(w, http.StatusBadGateway, "wasender unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// GET /api/wa/session — list sessions for the org's configured PAT.
// Returns WaSender's raw {success, data:[…]} shape unchanged so the
// frontend can pick the first session, render its status, and use its
// id for QR-related calls. We also fold in a hint about which session
// "looks current" so the UI doesn't have to guess if multiple exist.
func (s *Server) waListSessions(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	s.proxyToWaSender(w, r, http.MethodGet, "/api/whatsapp-sessions", pat, nil)
}

// POST /api/wa/session/{id}/connect — kick off connection for a session.
// WaSender returns the first QR code in the response body. The QR is a
// raw string (e.g. "2@DTMUH…"); the frontend turns it into an image with
// qrcode.react. The QR expires after 45s — clients should call the qr
// endpoint below to refresh.
//
// Side-effect: we also push our webhook URL up to WaSender (best-effort)
// so the moment the user finishes the QR scan, inbound messages start
// flowing back to this backend. Sync failures don't block the connect
// response — the operator can still scan, and the explicit sync-webhook
// button surfaces the error if it fails.
func (s *Server) waConnectSession(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	ac := getAuth(r)
	if err := s.syncWaSenderWebhook(r.Context(), ac.OrgID, id, pat); err != nil {
		// Logged but non-fatal — see comment above.
		s.logger.Sugar().Warnw("wa connect: webhook sync failed (continuing)",
			"session", id, "err", err.Error())
	}
	s.proxyToWaSender(w, r, http.MethodPost,
		fmt.Sprintf("/api/whatsapp-sessions/%s/connect", id), pat, nil)
}

// GET /api/wa/session/{id}/qr — fetch a fresh QR for a session that's
// already been initialized. Used for the auto-refresh polling loop on
// the frontend (every ~40s while status=NEED_SCAN).
func (s *Server) waSessionQR(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	s.proxyToWaSender(w, r, http.MethodGet,
		fmt.Sprintf("/api/whatsapp-sessions/%s/qrcode", id), pat, nil)
}

// POST /api/wa/session/{id}/sync-webhook — push our public webhook URL +
// secret to WaSender for this session. Without this step, WaSender never
// learns where to POST inbound messages, so the AI agent never sees a
// customer's "hlo" arriving on real WhatsApp.
//
// Idempotent: safe to call repeatedly. Also invoked automatically after a
// successful connect (see waConnectSession below) so the operator never
// has to think about it. We derive the URL from cfg.PublicServerURL so
// the same code works on testgo / testgo1 / local-ngrok without a
// per-env override.
func (s *Server) waSyncWebhook(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	ac := getAuth(r)
	if err := s.syncWaSenderWebhook(r.Context(), ac.OrgID, id, pat); err != nil {
		writeError(w, http.StatusBadGateway, "sync to WaSender failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"synced":      true,
		"webhook_url": s.cfg.PublicServerURL + "/wa/webhook/wasender",
	})
}

// syncWaSenderWebhook PUTs our public webhook URL and stored secret to
// WaSender's session-update endpoint. Used by the explicit sync-webhook
// route and by the post-connect auto-sync. Returns nil on 2xx, otherwise
// the WaSender error body so the operator can see what went wrong.
func (s *Server) syncWaSenderWebhook(ctx context.Context, orgID int64, sessionID, pat string) error {
	publicURL := strings.TrimRight(s.cfg.PublicServerURL, "/")
	if publicURL == "" || strings.HasPrefix(publicURL, "http://localhost") {
		return fmt.Errorf("PUBLIC_SERVER_URL must be a public HTTPS URL — got %q", publicURL)
	}
	webhookURL := publicURL + "/wa/webhook/wasender"

	// Look up the stored secret so WaSender signs every inbound with the
	// same value our verifier expects. If the org never set one, we send
	// an empty string — WaSender accepts this and the verifier falls back
	// to legacy "accept anything" mode (see verifyWaSenderSignature).
	secret := ""
	if cfg, _ := s.db.GetSingleActiveWAChannelConfig("wasender"); cfg != nil {
		secret = cfg.WebhookSecret
	}

	body, _ := json.Marshal(map[string]any{
		"webhook_url":     webhookURL,
		"webhook_enabled": true,
		"webhook_events":  []string{"messages.received", "session.status"},
		"webhook_secret":  secret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		wasenderBaseURL+"/api/whatsapp-sessions/"+sessionID, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := waSenderHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(buf))
	}
	s.logger.Sugar().Infow("wa session: synced webhook to WaSender",
		"org", orgID, "session", sessionID, "webhook_url", webhookURL)
	return nil
}

// POST /api/wa/session/{id}/disconnect — force WaSender to drop the
// session's WhatsApp link. The dashboard exposes this as a "Disconnect &
// Re-scan" button for the case where WaSender's reported status is
// stale (says "connected" while the user's phone has actually unlinked
// the device — happens after force-quit / network-loss / unclean
// logout). After hitting this, the session flips back to NEED_SCAN and
// the QR flow can mint a fresh link.
func (s *Server) waDisconnectSession(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	s.proxyToWaSender(w, r, http.MethodPost,
		fmt.Sprintf("/api/whatsapp-sessions/%s/disconnect", id), pat, nil)
}

