package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
)

func validPlivoPublicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// Plivo supplies international numbers. Accept the common Indian domestic
// formatting used by existing saved accounts as well, without guessing a DID
// from an arbitrary substring or accepting an empty/anonymous destination.
func plivoNumber(raw string) string {
	var digits strings.Builder
	for _, c := range strings.TrimSpace(raw) {
		switch {
		case c >= '0' && c <= '9':
			digits.WriteRune(c)
		case strings.ContainsRune("+ ()-", c):
		default:
			return ""
		}
	}
	number := strings.TrimPrefix(digits.String(), "00")
	if len(number) == 11 && number[0] == '0' {
		number = number[1:]
	}
	if len(number) == 10 {
		number = "91" + number
	}
	if len(number) < 8 || len(number) > 15 {
		return ""
	}
	return number
}

func plivoInboundIdentity(account *db.OrgExotelAccount, form url.Values) (contextID, phone string, err error) {
	if account == nil || account.Provider != "plivo" || account.ID <= 0 || account.OrgID <= 0 {
		return "", "", fmt.Errorf("invalid Plivo account")
	}
	did := plivoNumber(account.CallerID)
	if form.Get("Direction") != "inbound" || did == "" || plivoNumber(form.Get("To")) != did {
		return "", "", fmt.Errorf("incoming number does not match provider account")
	}
	call := form.Get("CallUUID")
	if call == "" || len(call) > 80 {
		return "", "", fmt.Errorf("invalid call UUID")
	}
	for _, c := range call {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return "", "", fmt.Errorf("invalid call UUID")
		}
	}
	caller := plivoNumber(form.Get("From"))
	if caller != "" {
		phone = "+" + caller
	}
	return fmt.Sprintf("plivo_in_%d_%s", account.ID, call), phone, nil
}

// Incoming webhooks are distinct from outbound answers: no lead, campaign or
// RequestUUID is required. Tenant identity comes from a DB account, its signed
// callback URL, and a matching called number, never from unsigned form fields.
func (s *Server) plivoInbound(w http.ResponseWriter, r *http.Request) {
	account, ok := s.verifyPlivo(w, r)
	if !ok {
		return
	}
	contextID, phone, err := plivoInboundIdentity(account, r.PostForm)
	if err != nil {
		http.Error(w, "Invalid incoming call", http.StatusForbidden)
		return
	}
	if !validPlivoPublicURL(s.cfg.PublicServerURL) {
		http.Error(w, "HTTPS callback URL required", http.StatusServiceUnavailable)
		return
	}
	if err := s.db.EnsurePlivoInboundCall(r.Context(), account.ID, account.OrgID, contextID, phone); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, db.ErrPlivoInboundContext) {
			status = http.StatusConflict
		}
		http.Error(w, "Incoming call unavailable", status)
		return
	}
	route, err := s.db.GetIncomingRoute(account.OrgID, account.ID)
	if err == nil {
		route, err = s.db.SnapshotIncomingRoute(r.Context(), account.OrgID, contextID, route)
	}
	if err != nil {
		http.Error(w, "Routing unavailable", http.StatusServiceUnavailable)
		return
	}
	if route.Mode == "phone" {
		accounts, lookupErr := s.db.GetOrgExotelAccounts(account.OrgID)
		if lookupErr != nil {
			http.Error(w, "Routing unavailable", http.StatusServiceUnavailable)
			return
		}
		for _, ownAccount := range accounts {
			if plivoNumber(ownAccount.CallerID) == plivoNumber(route.Phone) {
				http.Error(w, "Forwarding loop rejected", http.StatusConflict)
				return
			}
		}
		if plivoNumber(route.Phone) == plivoNumber(phone) {
			http.Error(w, "Forwarding loop rejected", http.StatusConflict)
			return
		}
		if err := writePlivoForward(w, account.CallerID, route.Phone); err != nil {
			http.Error(w, "Invalid forwarding destination", http.StatusServiceUnavailable)
		}
		return
	}
	if route.Mode != "ai" {
		http.Error(w, "Invalid incoming route", http.StatusServiceUnavailable)
		return
	}
	credit, err := s.db.GetOrgCredit(account.OrgID)
	if err != nil || credit == nil || credit.BalancePaise <= 0 {
		http.Error(w, "Incoming service unavailable", http.StatusServiceUnavailable)
		return
	}
	voice, err := s.db.GetOrganizationVoiceSettings(account.OrgID)
	if err != nil {
		http.Error(w, "Voice settings unavailable", http.StatusServiceUnavailable)
		return
	}
	call := r.PostForm.Get("CallUUID")
	pending := rstore.PendingCallInfo{
		IsInbound: true, PlivoContextID: contextID, ExotelCallSid: call,
		OrgID: account.OrgID, ExotelAccountID: account.ID, Phone: phone,
		Interest: "inbound enquiry", TTSProvider: voice.TTSProvider,
		TTSVoiceID: voice.TTSVoiceID, TTSLanguage: voice.TTSLanguage,
		MaxCallDurationSeconds: voice.MaxCallDurationSeconds,
	}
	if err := s.store.SetPendingCall(r.Context(), contextID, pending); err != nil {
		http.Error(w, "Call context unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.store.SetPendingCall(r.Context(), call, pending); err != nil {
		http.Error(w, "Call context unavailable", http.StatusServiceUnavailable)
		return
	}
	s.writePlivoStream(w, r, call, contextID, pending)
}

func (s *Server) plivoInboundStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := s.verifyPlivo(w, r)
	if !ok {
		return
	}
	contextID, _, err := plivoInboundIdentity(account, r.PostForm)
	if err != nil {
		http.Error(w, "Invalid incoming call", http.StatusForbidden)
		return
	}
	status := strings.ToLower(r.PostForm.Get("CallStatus"))
	switch status {
	case "completed", "failed", "busy", "no-answer", "canceled":
	default:
		w.WriteHeader(http.StatusOK)
		return
	}
	// This path deliberately does not schedule an outbound retry or change an
	// unrelated lead's disposition. Late callbacks work after Redis cleanup.
	if err := s.db.UpdatePlivoInboundStatus(r.Context(), account.OrgID, contextID, status); err != nil {
		http.Error(w, "Call status unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.wsHandler != nil {
		s.wsHandler.CloseCall(contextID)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) plivoInboundSetup(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "provider_accounts.global") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	account, err := s.db.GetExotelAccountByIDInOrg(id, ac.OrgID)
	if err != nil || account == nil || account.Provider != "plivo" {
		writeError(w, http.StatusNotFound, "Plivo account not found")
		return
	}
	if !validPlivoPublicURL(s.cfg.PublicServerURL) {
		writeError(w, http.StatusServiceUnavailable, "Configure a public HTTPS server URL first")
		return
	}
	query := url.Values{"org_id": {strconv.FormatInt(account.OrgID, 10)}, "account_id": {strconv.FormatInt(account.ID, 10)}}
	base := strings.TrimRight(s.cfg.PublicServerURL, "/")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{
		"number": account.CallerID, "answer_url": base + "/webhook/plivo/inbound?" + query.Encode(),
		"hangup_url": base + "/webhook/plivo/inbound/status?" + query.Encode(), "method": "POST",
	})
}
