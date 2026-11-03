package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"fmt"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	plivo "github.com/plivo/plivo-go/v7"
)

func (s *Server) verifyPlivo(w http.ResponseWriter, r *http.Request) (*db.OrgExotelAccount, bool) {
	org, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)
	id, _ := strconv.ParseInt(r.URL.Query().Get("account_id"), 10, 64)
	if org <= 0 || id <= 0 || len(r.URL.Query()["org_id"]) != 1 || len(r.URL.Query()["account_id"]) != 1 {
		http.Error(w, "Invalid provider account", http.StatusForbidden)
		return nil, false
	}
	if s.db == nil {
		http.Error(w, "Provider service unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	account, err := s.db.GetExotelAccountByIDInOrg(id, org)
	if err != nil || account == nil || account.Provider != "plivo" {
		http.Error(w, "Invalid provider account", 403)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	if r.ParseForm() != nil {
		http.Error(w, "Invalid form", 400)
		return nil, false
	}
	params := map[string]string{}
	for k, v := range r.PostForm {
		if len(v) != 1 {
			http.Error(w, "Ambiguous form", 400)
			return nil, false
		}
		params[k] = v[0]
	}
	if !validPlivoSignature(r, s.cfg.PublicServerURL, account.APIToken, params) {
		http.Error(w, "Invalid Plivo signature", 403)
		return nil, false
	}
	return account, true
}

func validPlivoSignature(r *http.Request, publicURL, token string, params map[string]string) bool {
	nonce, signature := r.Header.Get("X-Plivo-Signature-V3-Nonce"), r.Header.Get("X-Plivo-Signature-V3")
	if nonce == "" || signature == "" || token == "" {
		return false
	}
	endpoint := strings.TrimRight(publicURL, "/") + r.URL.RequestURI()
	return plivo.ValidateSignatureV3(endpoint, nonce, r.Method, signature, token, params)
}

func (s *Server) plivoAnswer(w http.ResponseWriter, r *http.Request) {
	account, ok := s.verifyPlivo(w, r)
	if !ok {
		return
	}
	call, request := r.PostForm.Get("CallUUID"), r.PostForm.Get("RequestUUID")
	contextID := r.URL.Query().Get("context_id")
	pending, err := bindPlivoCall(r.Context(), s.store, account, contextID, call, request)
	if err != nil {
		http.Error(w, "Call context unavailable", 409)
		return
	}
	if err := s.db.UpdateCallLogStatus(contextID, "in-progress"); err != nil {
		http.Error(w, "Call status unavailable", 503)
		return
	}
	s.writePlivoStream(w, r, call, contextID, pending)
}

func (s *Server) writePlivoStream(w http.ResponseWriter, r *http.Request, call, contextID string, pending rstore.PendingCallInfo) {
	if !validPlivoPublicURL(s.cfg.PublicServerURL) {
		http.Error(w, "HTTPS callback URL required", http.StatusServiceUnavailable)
		return
	}
	ticket, err := dial.PlivoStreamTicket(s.cfg.JWTSecret, call, contextID)
	if err != nil {
		http.Error(w, "Stream unavailable", 500)
		return
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		http.Error(w, "Stream unavailable", 500)
		return
	}
	key := hex.EncodeToString(nonce)
	if err = s.store.SetPendingCall(r.Context(), "plivo-ticket:"+key, pending); err != nil {
		http.Error(w, "Stream unavailable", 503)
		return
	}
	base := strings.Replace(strings.TrimRight(s.cfg.PublicServerURL, "/"), "https://", "wss://", 1)
	stream := base + "/media-stream/plivo?ticket=" + url.QueryEscape(ticket) + "&nonce=" + key
	response := struct {
		XMLName xml.Name `xml:"Response"`
		Stream  struct {
			Bidirectional string `xml:"bidirectional,attr"`
			KeepAlive     string `xml:"keepCallAlive,attr"`
			ContentType   string `xml:"contentType,attr"`
			URL           string `xml:",chardata"`
		} `xml:"Stream"`
	}{}
	response.Stream.Bidirectional = "true"
	response.Stream.KeepAlive = "true"
	response.Stream.ContentType = "audio/x-mulaw;rate=8000"
	response.Stream.URL = stream
	data, err := xml.Marshal(response)
	if err != nil {
		http.Error(w, "Stream unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(append([]byte(xml.Header), data...))
}

// The context exists before dialing; no dependency on the REST response UUID.
func bindPlivoCall(ctx context.Context, store *rstore.Store, account *db.OrgExotelAccount, contextID, call, request string) (rstore.PendingCallInfo, error) {
	pending, found := store.GetPendingCall(ctx, contextID)
	if call == "" || request == "" || !found || pending.PlivoContextID != contextID || pending.OrgID != account.OrgID || pending.ExotelAccountID != account.ID {
		return rstore.PendingCallInfo{}, fmt.Errorf("invalid Plivo call context")
	}
	if pending.ExotelCallSid != "" && pending.ExotelCallSid != call {
		return rstore.PendingCallInfo{}, fmt.Errorf("Plivo call identity changed")
	}
	pending.ExotelCallSid, pending.PlivoRequestID = call, request
	if err := store.SetPendingCall(ctx, contextID, pending); err != nil {
		return pending, err
	}
	return pending, store.SetPendingCall(ctx, call, pending)
}

func (s *Server) plivoStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := s.verifyPlivo(w, r)
	if !ok {
		return
	}
	contextID := r.URL.Query().Get("context_id")
	pending, found := s.store.GetPendingCall(r.Context(), contextID)
	if !found || pending.PlivoContextID != contextID || pending.OrgID != account.OrgID || pending.ExotelAccountID != account.ID {
		http.Error(w, "Unknown call", 409)
		return
	}
	if r.PostForm.Get("CallStatus") == "ringing" && pending.ExotelCallSid != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Ring callbacks must not make a pending request look like an answered call.
	if call := r.PostForm.Get("CallUUID"); call != "" && r.PostForm.Get("CallStatus") != "ringing" {
		pending.ExotelCallSid = call
		pending.PlivoRequestID = r.PostForm.Get("RequestUUID")
		if err := s.store.SetPendingCall(r.Context(), contextID, pending); err != nil {
			http.Error(w, "Call context unavailable", 503)
			return
		}
	}
	r.PostForm.Set("CallSid", contextID)
	r.Form.Set("CallSid", contextID)
	// Reuse existing outcome/event processing after provider signature verification.
	s.providerStatus(w, r, "Plivo")
}
