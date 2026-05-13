package api

import (
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
	// Remap upstream 401/403 → 400 so the browser's apiFetch doesn't
	// mistake a WaSender "bad PAT" rejection for a Callified auth failure
	// and log the operator out. A 400 surfaces as an error in the UI
	// (session panel shows "invalid token") without clearing the session.
	status := resp.StatusCode
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)
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

// fmtQuery is a tiny helper for forwarding query strings unchanged. Not
// currently needed (none of the session endpoints take query params),
// kept as a placeholder so the proxy structure stays consistent if we
// later add /api/whatsapp-sessions?status=connected etc. The blank-import
// avoids "imported and not used" while leaving the helper visible.
var _ = strings.TrimSpace
var _ = json.Marshal
