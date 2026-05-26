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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// GET /api/wa/session
// @Summary     List WaSender sessions
// @Description Returns WaSender session list for the org's Personal Access Token. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/session [get]
func (s *Server) waListSessions(w http.ResponseWriter, r *http.Request) {
	pat := s.waSessionPAT(w, r)
	if pat == "" {
		return
	}
	s.proxyToWaSender(w, r, http.MethodGet, "/api/whatsapp-sessions", pat, nil)
}

// POST /api/wa/session/{id}/connect
// @Summary     Connect WaSender session
// @Description Initiates connection and returns the initial QR code for scanning. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  string  true  "WaSender session ID"
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/session/{id}/connect [post]
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

// GET /api/wa/session/{id}/qr
// @Summary     Get WaSender session QR
// @Description Fetches a fresh QR code for an initialized session (used for 40s auto-refresh). Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  string  true  "WaSender session ID"
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/session/{id}/qr [get]
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

// POST /api/wa/session/{id}/disconnect
// @Summary     Disconnect WaSender session
// @Description Drops the WhatsApp link and resets session to NEED_SCAN state. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  string  true  "WaSender session ID"
// @Success     200  {object}  object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/session/{id}/disconnect [post]
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
