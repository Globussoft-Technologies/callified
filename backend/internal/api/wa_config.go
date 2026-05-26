package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/wa"
)

// GET /api/wa/channels
// @Summary     List WhatsApp channels
// @Description Returns all WhatsApp channel configurations for the org. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.WAChannelConfig
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/channels [get]
func (s *Server) listWAChannels(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	configs, err := s.db.GetWAChannelConfigsByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(configs))
}

// POST /api/wa/channels
// @Summary     Create WhatsApp channel
// @Description Adds a new WhatsApp channel configuration. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{provider=string,phone_number=string,api_key=string,app_id=string,webhook_url=string}  true  "Channel config"
// @Success     201  {object}  IDResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/channels [post]
func (s *Server) createWAChannel(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Provider    string `json:"provider"`
		PhoneNumber string `json:"phone_number"`
		APIKey      string `json:"api_key"`
		AppID       string `json:"app_id"`
		WebhookURL  string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" || body.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "provider and phone_number required")
		return
	}
	id, err := s.db.CreateWAChannelConfig(ac.OrgID, body.Provider, body.PhoneNumber, body.APIKey, body.AppID, body.WebhookURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// PUT /api/wa/channels/{id}
// @Summary     Update WhatsApp channel
// @Description Updates credentials/settings for an existing WhatsApp channel. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64  true  "Channel ID"
// @Param       body  body  object{api_key=string,app_id=string,webhook_url=string,ai_enabled=bool}  true  "Updated fields"
// @Success     200  {object}  object{updated=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/channels/{id} [put]
func (s *Server) updateWAChannel(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		APIKey     string `json:"api_key"`
		AppID      string `json:"app_id"`
		WebhookURL string `json:"webhook_url"`
		AIEnabled  bool   `json:"ai_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.UpdateWAChannelConfig(id, ac.OrgID, body.APIKey, body.AppID, body.WebhookURL, body.AIEnabled); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// DELETE /api/wa/channels/{id}
// @Summary     Delete WhatsApp channel
// @Description Removes a WhatsApp channel configuration. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  int64  true  "Channel ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/channels/{id} [delete]
func (s *Server) deleteWAChannel(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteWAChannelConfig(id, ac.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// PUT /api/wa/channels/{id}/toggle-ai
// @Summary     Toggle AI on WhatsApp channel
// @Description Enables or disables AI auto-reply for a specific channel. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64  true  "Channel ID"
// @Param       body  body  object{enabled=bool}  true  "AI enabled flag"
// @Success     200  {object}  object{ai_enabled=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/channels/{id}/toggle-ai [put]
func (s *Server) toggleWAAI(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.ToggleWAAI(id, ac.OrgID, body.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ai_enabled": body.Enabled})
}

// GET /api/wa/conversations
// @Summary     List WhatsApp conversations
// @Description Returns recent WhatsApp conversations for the org. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       archived  query  int  false  "Set to 1 to include archived conversations"
// @Success     200  {array}   db.WAConversationRow
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations [get]
func (s *Server) listWAConversations(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	includeArchived := r.URL.Query().Get("archived") == "1"
	convs, err := s.db.GetWAConversationsList(ac.OrgID, 50, includeArchived)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(convs))
}

// GET /api/wa/conversations/{id}/history
// @Summary     Get WhatsApp chat history
// @Description Returns the last 100 messages in a WhatsApp conversation. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  int64  true  "Conversation ID"
// @Success     200  {array}   db.WAMessage
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{id}/history [get]
func (s *Server) getWAHistory(w http.ResponseWriter, r *http.Request) {
	convID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	history, err := s.db.GetWAChatHistory(convID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(history))
}

// ── /api/wa/config ──────────────────────────────────────────────────────────
//
// Single-config-per-org compatibility shim for the frontend modal
// (WhatsAppTab.jsx). Python exposes /api/wa/config returning a shape
// like `{provider, credentials{}, default_product_id, auto_reply}` and the
// Go native endpoints work with flat columns under /api/wa/channels.
// These two handlers translate between the shapes so the existing UI works
// without a rewrite.

// GET /api/wa/config
// @Summary     Get WhatsApp config (legacy)
// @Description Returns the org's active WhatsApp channel config in the legacy single-config shape. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object{provider=string,credentials=object,default_product_id=int,auto_reply=bool}
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/config [get]
func (s *Server) getWAConfig(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	configs, err := s.db.GetWAChannelConfigsByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(configs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":           "gupshup",
			"credentials":        map[string]string{},
			"default_product_id": nil,
			"auto_reply":         true,
		})
		return
	}
	// Pick the *active* config — there may be multiple historical rows
	// from when the operator switched providers (each save upserts on
	// (org_id, provider) so the loser stays in the table). Prefer
	// is_active=1 first, then fall back to highest id (most recently
	// inserted). This makes the modal reflect the LAST saved state,
	// not the first historical one — and matches what the send path
	// will actually use.
	cfg := configs[0]
	for _, c := range configs {
		if c.IsActive {
			cfg = c
			break
		}
	}
	// Merge the JSON credentials column with the legacy flat columns so any
	// provider's full field set surfaces. Flat columns win on conflict for
	// backwards-compatibility with rows written before the JSON column was
	// wired through (those rows have flat values but `credentials='{}'`).
	creds := map[string]string{}
	for k, v := range cfg.Credentials {
		if v != "" {
			creds[k] = v
		}
	}
	if cfg.APIKey != "" {
		creds["api_key"] = cfg.APIKey
	}
	if cfg.AppID != "" {
		creds["app_id"] = cfg.AppID
	}
	if cfg.PhoneNumber != "" {
		creds["phone_number"] = cfg.PhoneNumber
	}
	// Pre-fill the webhook secret in the modal so the operator can see
	// they have one set without re-pasting it. Frontend treats empty as
	// "leave alone" on save (matches the COALESCE in UpsertWA).
	if cfg.WebhookSecret != "" {
		creds["webhook_secret"] = cfg.WebhookSecret
	}
	var defaultProd interface{}
	if cfg.DefaultProductID > 0 {
		defaultProd = cfg.DefaultProductID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 cfg.ID,
		"provider":           cfg.Provider,
		"credentials":        creds,
		"default_product_id": defaultProd,
		"auto_reply":         cfg.AIEnabled,
	})
}

// validWAProviders is the closed set of WA channel providers the backend
// supports. Bonus side-effect: rejecting unknown providers at save time means
// we never try to render a webhook URL or persist a config for a typo'd
// provider name that no provider-specific code path would ever read.
var validWAProviders = map[string]bool{
	"gupshup": true, "wati": true, "aisensei": true, "interakt": true, "meta": true, "wasender": true,
}

// POST /api/wa/config
// @Summary     Save WhatsApp config (legacy)
// @Description Upserts the org's WhatsApp channel config. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{provider=string,credentials=object,default_product_id=int,auto_reply=bool}  true  "WA config"
// @Success     200  {object}  object{saved=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/config [post]
func (s *Server) saveWAConfig(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	s.logger.Sugar().Infow("saveWAConfig called", "org_id", ac.OrgID)
	var body struct {
		Provider         string            `json:"provider"`
		Credentials      map[string]string `json:"credentials"`
		DefaultProductID *int64            `json:"default_product_id"`
		AutoReply        *bool             `json:"auto_reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider required")
		return
	}
	body.Provider = strings.TrimSpace(body.Provider)
	if !validWAProviders[body.Provider] {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	// Require at least one non-empty credential. The previous handler
	// happily upserted a row with all-empty {api_key, app_id, phone_number,
	// webhook_url} columns when the modal Save fired with blank inputs —
	// resulting in a "configured" channel that silently fails on the first
	// outbound send. Per-provider key validation is deferred (see #46
	// follow-up: backend reads `app_id`/`phone_number` while the gupshup UI
	// posts `app_name`/`source_phone`, so a strict check here would need
	// the key-name reconciliation first).
	hasAnyCred := false
	for _, v := range body.Credentials {
		if strings.TrimSpace(v) != "" {
			hasAnyCred = true
			break
		}
	}
	if !hasAnyCred {
		writeError(w, http.StatusBadRequest, "at least one credential is required")
		return
	}
	apiKey := body.Credentials["api_key"]
	appID := body.Credentials["app_id"]
	phone := body.Credentials["phone_number"]
	webhookURL := body.Credentials["webhook_url"]
	// webhook_secret is the shared-secret WaSender (and other providers
	// using a similar scheme) sends in X-Webhook-Signature on every
	// inbound delivery. Stored separately from the JSON credentials blob
	// so the webhook handler can read it cheaply without parsing JSON
	// per request. Empty value means "don't verify" (backwards-compat
	// for configs saved before this column existed).
	webhookSecret := body.Credentials["webhook_secret"]

	// UNIQUE(org_id, provider) turns this INSERT into an upsert. Avoids the
	// need for a separate update path + lookup.
	rowID, err := s.db.UpsertWAChannelConfig(ac.OrgID, body.Provider, phone, apiKey, appID, webhookURL, webhookSecret, body.Credentials, body.AutoReply, body.DefaultProductID)
	if err != nil {
		s.logger.Sugar().Errorw("saveWAConfig upsert failed", "org_id", ac.OrgID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.logger.Sugar().Infow("saveWAConfig succeeded", "org_id", ac.OrgID, "provider", body.Provider, "row_id", rowID)
	// One active provider per org. When the operator switches providers,
	// the new row gets upserted above and the OTHER provider rows for
	// the same org get deactivated here. Without this, sends would keep
	// going through whichever historical row had a lower id — leading
	// to "I saved WaSender but messages still go through Meta" surprises.
	if err := s.db.DeactivateOtherWAChannelConfigs(ac.OrgID, body.Provider); err != nil {
		s.logger.Sugar().Warnw("saveWAConfig: deactivate others failed", "err", err)
		// non-fatal — the save itself succeeded
	}
	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}

// GET /api/wa/conversations/{phone}/messages
// @Summary     Get messages by phone
// @Description Returns chat messages for a phone number's WhatsApp conversation. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number (with or without +)"
// @Success     200  {array}   db.WAMessage
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{phone}/messages [get]
func (s *Server) getWAMessagesByPhone(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := r.PathValue("phone")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	convID, err := s.db.GetWAConversationIDByPhone(ac.OrgID, phone)
	if err != nil || convID == 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	history, err := s.db.GetWAChatHistory(convID, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(history))
}

// POST /api/wa/toggle-ai/{phone}
// @Summary     Toggle AI by phone
// @Description Enables or disables AI auto-reply for a specific WhatsApp conversation. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number"
// @Param       body   body  object{enabled=bool}  true  "AI enabled flag"
// @Success     200  {object}  object{ai_enabled=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/toggle-ai/{phone} [post]
func (s *Server) toggleWAAIByPhone(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := r.PathValue("phone")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.ToggleWAConversationAI(ac.OrgID, phone, body.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ai_enabled": body.Enabled})
}

// POST /api/wa/send
// @Summary     Send WhatsApp message
// @Description Sends a manual outbound WhatsApp message. Accepts to_phone/message or contact_phone/text. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{to_phone=string,message=string,channel_id=int64}  true  "Message details"
// @Success     200  {object}  object{sent=bool,conversation_id=int64,phone=string}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/send [post]
func (s *Server) sendWAMessage(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		ChannelID    int64  `json:"channel_id"`
		ToPhone      string `json:"to_phone"`
		ContactPhone string `json:"contact_phone"`
		Message      string `json:"message"`
		Text         string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	phone := strings.TrimSpace(firstNonEmpty(body.ToPhone, body.ContactPhone))
	text := strings.TrimSpace(firstNonEmpty(body.Message, body.Text))
	if phone == "" || text == "" {
		writeError(w, http.StatusBadRequest, "phone and text required")
		return
	}

	settings, err := s.db.GetWAChannelConfigsByOrg(ac.OrgID)
	if err != nil || len(settings) == 0 {
		writeError(w, http.StatusBadRequest, "no WA channel configured")
		return
	}

	// Pick the *active* config — matches the getWAConfig logic so the
	// modal and the send path agree about which provider is in use.
	// Without this filter, the send path would always use the lowest-id
	// row (probably an old provider the operator already switched away
	// from), causing "I saved WaSender but messages go via Meta" surprises.
	cfg := settings[0]
	for _, c := range settings {
		if c.IsActive {
			cfg = c
			break
		}
	}
	if body.ChannelID > 0 {
		for _, c := range settings {
			if c.ID == body.ChannelID {
				cfg = c
				break
			}
		}
	}

	channelCfg := s.waChannelConfig(cfg.OrgID, cfg.Provider, cfg.PhoneNumber, cfg.APIKey, cfg.AppID, cfg.DefaultProductID)
	if err := s.waSender.SendText(r.Context(), channelCfg, phone, text); err != nil {
		// Surface the provider's actual error so the frontend can display
		// it instead of a misleading "HTTP 502". Most provider errors are
		// auth/quota/allow-list issues the operator can fix immediately
		// (rotate the token, add the recipient to allow-list, top up the
		// account). Hiding them behind 502 made the dashboard useless for
		// diagnosing real provider failures. Status code stays 502 — the
		// upstream did fail — but the body carries the readable detail.
		s.logger.Sugar().Warnw("wa send failed", "provider", cfg.Provider, "to", phone, "err", err.Error())
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Persist the outbound message so it appears in the dashboard chat
	// view immediately. Normalize to E.164-without-plus so the stored key
	// matches whatever format inbound webhooks use (WaSender always sends
	// full international digits like "917795740488"; a 10-digit dashboard
	// input must be expanded to the same form or they create two rows).
	storedPhone := strings.TrimPrefix(wa.NormalizePhone(phone), "+")
	convID, err := s.db.GetOrCreateWAConversation(cfg.OrgID, storedPhone, cfg.Provider)
	if err == nil && convID > 0 {
		_, _ = s.db.SaveWAMessage(convID, "outbound", text, "text", "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "conversation_id": convID, "phone": storedPhone})
}

// firstNonEmpty returns the first argument that's not the empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// POST /api/wa/conversations/ensure
// @Summary     Ensure WhatsApp conversation
// @Description Idempotently creates a conversation row for the given phone. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{phone=string,provider=string}  true  "Phone and provider"
// @Success     200  {object}  object{conversation_id=int64,phone=string}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/ensure [post]
func (s *Server) ensureWAConversation(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Phone    string `json:"phone"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	phone := strings.TrimPrefix(strings.TrimSpace(body.Phone), "+")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	provider := strings.TrimSpace(body.Provider)
	if provider == "" {
		// Fall back to the org's first configured provider so the
		// caller doesn't need to know which one was set up.
		if cfgs, _ := s.db.GetWAChannelConfigsByOrg(ac.OrgID); len(cfgs) > 0 {
			provider = cfgs[0].Provider
		} else {
			provider = "wasender" // last-resort default
		}
	}
	convID, err := s.db.GetOrCreateWAConversation(ac.OrgID, phone, provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation_id": convID, "phone": phone})
}

// ─── Conversation management endpoints ───
// All four resolve the phone from the URL path, scope the DB write by
// (org_id, phone) so a malicious client can't reach into another org's
// data, and return a tiny JSON ack the dashboard polls before
// re-rendering the inbox.

// normalizePathPhone strips an optional leading + from a phone path
// segment. The dashboard sometimes URL-encodes the +, sometimes drops
// it; the DB stores phones without it. Always normalise here so the
// handlers don't have to care which shape they received.
func normalizePathPhone(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "+")
}

// POST /api/wa/conversations/{phone}/mute
// @Summary     Mute/unmute WhatsApp conversation
// @Description Suppresses or restores AI auto-reply for one thread. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number"
// @Param       body   body  object{muted=bool}  true  "Muted flag"
// @Success     200  {object}  object{muted=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{phone}/mute [post]
func (s *Server) muteWAConversation(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := normalizePathPhone(r.PathValue("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	var body struct {
		Muted bool `json:"muted"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.db.SetWAConversationMuted(ac.OrgID, phone, body.Muted); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"muted": body.Muted})
}

// POST /api/wa/conversations/{phone}/archive
// @Summary     Archive/unarchive WhatsApp conversation
// @Description Hides or shows a thread in the default inbox. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number"
// @Param       body   body  object{archived=bool}  true  "Archived flag"
// @Success     200  {object}  object{archived=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{phone}/archive [post]
func (s *Server) archiveWAConversation(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := normalizePathPhone(r.PathValue("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	var body struct {
		Archived bool `json:"archived"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.db.SetWAConversationArchived(ac.OrgID, phone, body.Archived); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"archived": body.Archived})
}

// POST /api/wa/conversations/{phone}/clear
// @Summary     Clear WhatsApp conversation
// @Description Wipes message history while keeping the conversation row. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number"
// @Success     200  {object}  object{cleared=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{phone}/clear [post]
func (s *Server) clearWAConversation(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := normalizePathPhone(r.PathValue("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	if err := s.db.ClearWAConversationMessages(ac.OrgID, phone); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cleared": true})
}

// DELETE /api/wa/conversations/{phone}
// @Summary     Delete WhatsApp conversation
// @Description Permanently removes a conversation and all its messages. Requires Admin role.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "Phone number"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/wa/conversations/{phone} [delete]
func (s *Server) deleteWAConversation(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := normalizePathPhone(r.PathValue("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	if err := s.db.DeleteWAConversation(ac.OrgID, phone); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
