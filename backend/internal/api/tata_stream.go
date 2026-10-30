package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
)

func tataSecretHash(secret string) string {
	v := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(v[:])
}
func (s *Server) rotateTataStreamSecret(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	account, err := s.db.GetOrgExotelAccountByID(id, ac.OrgID)
	if err != nil || account == nil {
		writeError(w, 404, "account not found")
		return
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		writeError(w, 500, "cannot generate secret")
		return
	}
	secret := hex.EncodeToString(b)
	if err = s.db.SetTataStreamSecret(id, ac.OrgID, tataSecretHash(secret)); err != nil {
		writeError(w, 400, "cannot configure Tata account")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"secret": secret, "header": "X-Callified-Secret", "url": strings.TrimRight(s.cfg.PublicServerURL, "/") + "/webhook/tata/stream-url"})
}
func (s *Server) tataStreamURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	secret := r.Header.Get("X-Callified-Secret")
	if len(secret) != 64 {
		writeError(w, 401, "invalid authentication")
		return
	}
	account, err := s.db.GetTataAccountByStreamSecret(ctx, tataSecretHash(secret))
	if err != nil || account == nil {
		writeError(w, 401, "invalid authentication")
		return
	}
	// Record possession of the current secret even when Tata's dashboard test uses sample call details.
	if err := s.db.MarkTataStreamVerified(ctx, account.ID, account.OrgID, tataSecretHash(secret)); err != nil {
		writeError(w, 409, "secret changed; verify again")
		return
	}
	s.resolveTataStreamURL(ctx, w, r, account)
}
func (s *Server) resolveTataStreamURL(ctx context.Context, w http.ResponseWriter, r *http.Request, account *db.OrgExotelAccount) {
	var body struct {
		CallID string `json:"callId"`
		From   string `json:"fromNumber"`
		To     string `json:"toNumber"`
		Status string `json:"status"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.CallID) == "" || len(body.CallID) > 200 || !validTataNumber(body.From) || !validTataNumber(body.To) {
		writeError(w, 400, "invalid call details")
		return
	}
	inbound := account.Direction == "inbound"
	did := body.From
	if inbound {
		did = body.To
	}
	if !sameTataNumber(did, account.CallerID) {
		writeError(w, 403, "DID does not belong to account")
		return
	}
	base, err := url.Parse(s.cfg.PublicServerURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		writeError(w, 503, "stream unavailable")
		return
	}
	pending := rstore.PendingCallInfo{OrgID: account.OrgID, Phone: body.From, ExotelAccountID: account.ID}
	if !inbound {
		pending, err = s.store.GetTataDial(ctx, account.ID, body.To)
		if err != nil || pending.OrgID != account.OrgID {
			writeError(w, 409, "no matching outbound call")
			return
		}
	}
	info := rstore.TataStreamContext{CallID: body.CallID, From: body.From, To: body.To, AccountID: account.ID, Inbound: inbound, Pending: pending}
	ticket, err := s.store.IssueTataStreamTicket(ctx, info)
	if err != nil {
		writeError(w, 503, "stream unavailable")
		return
	}
	base.Scheme = "wss"
	base.Path = "/media-stream/tata"
	base.RawQuery = url.Values{"ticket": {ticket}}.Encode()
	base.Fragment = ""
	writeJSON(w, 200, map[string]any{"success": true, "wss_url": base.String()})
}
func validTataNumber(v string) bool {
	v = rstore.NormalizeTataNumber(v)
	if len(v) < 7 || len(v) > 15 {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func sameTataNumber(a, b string) bool {
	return rstore.NormalizeTataNumber(a) == rstore.NormalizeTataNumber(b)
}

func (s *Server) tataStreamReadiness(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	ready, err := s.db.TataStreamReady(r.Context(), id, getAuth(r).OrgID)
	if err != nil {
		writeError(w, 404, "Tata account not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"verified": ready})
}
