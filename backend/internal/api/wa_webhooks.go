package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/wa"
)

// POST /wa/webhook/gupshup
// @Summary     Gupshup inbound webhook
// @Description Receives inbound WhatsApp messages from Gupshup. Called by the Gupshup platform.
// @Tags        webhooks
// @Accept      json
// @Success     200  "OK"
// @Router      /wa/webhook/gupshup [post]
func (s *Server) waWebhookGupshup(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "gupshup", wa.ParseGupshup)
}

// POST /wa/webhook/wati
// @Summary     Wati inbound webhook
// @Description Receives inbound WhatsApp messages from Wati. Called by the Wati platform.
// @Tags        webhooks
// @Accept      json
// @Success     200  "OK"
// @Router      /wa/webhook/wati [post]
func (s *Server) waWebhookWati(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "wati", wa.ParseWati)
}

// POST /wa/webhook/aisensei
// @Summary     AiSensei inbound webhook
// @Description Receives inbound WhatsApp messages from AiSensei. Called by the AiSensei platform.
// @Tags        webhooks
// @Accept      json
// @Success     200  "OK"
// @Router      /wa/webhook/aisensei [post]
func (s *Server) waWebhookAiSensei(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "aisensei", wa.ParseAiSensei)
}

// POST /wa/webhook/interakt
// @Summary     Interakt inbound webhook
// @Description Receives inbound WhatsApp messages from Interakt. Called by the Interakt platform.
// @Tags        webhooks
// @Accept      json
// @Success     200  "OK"
// @Router      /wa/webhook/interakt [post]
func (s *Server) waWebhookInterakt(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "interakt", wa.ParseInterakt)
}

// POST /wa/webhook/wasender
// @Summary     WaSender inbound webhook
// @Description Receives inbound WhatsApp messages from WaSender (X-Webhook-Signature verified). Called by WaSender.
// @Tags        webhooks
// @Accept      json
// @Success     200  "OK"
// @Failure     401  {object}  ErrorResponse
// @Router      /wa/webhook/wasender [post]
func (s *Server) waWebhookWaSender(w http.ResponseWriter, r *http.Request) {
	if !s.verifyWaSenderSignature(r) {
		s.logger.Sugar().Warnw("waWebhookWaSender: signature mismatch — request dropped",
			"remote", r.RemoteAddr, "header_present", r.Header.Get("X-Webhook-Signature") != "")
		writeError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	s.handleWAWebhook(w, r, "wasender", wa.ParseWaSender)
}

// verifyWaSenderSignature returns true when:
//   - we have no secret configured (legacy / opt-in mode), OR
//   - X-Webhook-Signature exactly matches our stored webhook_secret.
//
// We pull the secret from the single active wasender config; multi-
// config orgs aren't supported yet (the rest of the WaSender code path
// also assumes one session per backend), so the "first match" lookup
// is fine. If somebody adds multi-config later, this will need a per-
// channel routing layer first.
func (s *Server) verifyWaSenderSignature(r *http.Request) bool {
	cfg, err := s.db.GetSingleActiveWAChannelConfig("wasender")
	if err != nil || cfg == nil || cfg.WebhookSecret == "" {
		// No secret stored → can't verify, accept anything (legacy).
		return true
	}
	header := r.Header.Get("X-Webhook-Signature")
	return header == cfg.WebhookSecret
}

// POST /wa/webhook/meta
// @Summary     Meta WhatsApp webhook
// @Description Receives inbound messages from Meta Cloud API (POST) or hub challenge verification (GET).
// @Tags        webhooks
// @Accept      json
// @Param       hub.mode          query  string  false  "Challenge verification mode"
// @Param       hub.verify_token  query  string  false  "Verification token"
// @Param       hub.challenge     query  string  false  "Challenge string"
// @Success     200  "OK or challenge echo"
// @Failure     403  {object}  ErrorResponse
// @Router      /wa/webhook/meta [post]
func (s *Server) waWebhookMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Hub challenge verification
		mode := r.URL.Query().Get("hub.mode")
		token := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")
		if mode == "subscribe" && token == s.cfg.MetaVerifyToken && challenge != "" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(challenge))
			return
		}
		writeError(w, http.StatusForbidden, "verification failed")
		return
	}
	s.handleWAWebhook(w, r, "meta", wa.ParseMeta)
}

// handleWAWebhook is the shared handler for all inbound WA provider webhooks.
func (s *Server) handleWAWebhook(w http.ResponseWriter, r *http.Request, provider string,
	parser func([]byte) (*wa.IncomingMessage, error)) {

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	msg, err := parser(body)
	if err != nil || msg == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Look up the channel config by provider + destination phone. cfg may be
	// nil if the provider points at a number we don't own — still log the
	// message so the operator can see orphaned traffic in the DB, but
	// against org_id=0 (only truly unattributable case).
	cfg, _ := s.db.GetWAChannelConfigByPhone(provider, msg.ToPhone)
	// Some providers (WaSender) don't include the destination phone in the
	// inbound webhook payload. Fall back: if there's exactly one active
	// config for this provider, use it. Multi-tenant ambiguity is impossible
	// here because each (org, provider) is unique by schema and a single
	// org's WaSender device only ever has one number.
	if cfg == nil {
		cfg, _ = s.db.GetSingleActiveWAChannelConfig(provider)
	}
	var orgID int64
	if cfg != nil {
		orgID = cfg.OrgID
	}

	// Skip empty-phone events. Some provider event types (presence
	// updates, status acks, group metadata) come through this webhook
	// without a from-phone. Without this guard, every such event would
	// create an orphan conversation row keyed on phone='' that bleeds
	// into the inbox as a row with no name or content.
	if strings.TrimSpace(msg.FromPhone) == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Dedup: providers retry deliveries when our 200 response is delayed.
	// If a message with this provider_msg_id already exists, do not re-run
	// the AI agent for it — return 200 so the provider stops retrying.
	if msg.ProviderMsgID != "" {
		if existing, _ := s.db.GetWAMessageByProviderID(msg.ProviderMsgID); existing != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Normalize to E.164-without-plus so inbound webhooks and dashboard
	// outbound sends write the same key into whatsapp_conversations.phone.
	// WaSender includes the country code in the JID ("917795740488"); a
	// bare 10-digit dashboard send would create a second orphan row without
	// this step. Mutate msg.FromPhone so the agent's own DB lookup agrees.
	msg.FromPhone = strings.TrimPrefix(wa.NormalizePhone(msg.FromPhone), "+")
	fromPhone := msg.FromPhone

	// Always persist the inbound message before any AI processing so the
	// Inbox shows it. Previously the AI branch skipped the save entirely,
	// and the non-AI branch saved with org_id=0 which orphaned the row
	// (every org's /api/wa/conversations filters by the authed user's
	// org_id, so org_id=0 rows are invisible to everyone). Empty-text
	// messages (rare — typically "media-only" with parse drop) still
	// create the conversation row but skip the message-save so the
	// thread can receive a future text without a leading blank bubble.
	convID, _ := s.db.GetOrCreateWAConversation(orgID, fromPhone, provider)
	if convID > 0 && strings.TrimSpace(msg.Text) != "" {
		_, _ = s.db.SaveWAMessage(convID, "inbound", msg.Text, msg.MessageType, msg.ProviderMsgID)
	}

	if cfg == nil || !cfg.AIEnabled {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Per-conversation mute: even if the channel-wide AI is on, an
	// operator can mute a specific thread (handed off for manual reply,
	// or VIP customer who shouldn't get a bot). Inbound is still saved
	// above, but the AI branch is skipped.
	if s.db.IsWAConversationMuted(cfg.OrgID, fromPhone) {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process with AI agent (async so we return 200 quickly). The goroutine
	// must NOT inherit r.Context() — that gets canceled the moment we
	// write the 200 response, which would kill the LLM request mid-stream.
	// Detach with a fresh background context bounded by a sane timeout.
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go func() {
		defer cancel()
		if s.waAgent == nil {
			return
		}
		channelCfg := s.waChannelConfig(cfg.OrgID, cfg.Provider, cfg.PhoneNumber, cfg.APIKey, cfg.AppID, cfg.DefaultProductID)
		reply, err := s.waAgent.ProcessIncoming(bgCtx, channelCfg, msg)
		if err != nil {
			s.logger.Warn("waWebhook: agent failed",
				zap.String("provider", provider), zap.Error(err))
			return
		}
		if reply == "" {
			return
		}
		if err := s.waSender.SendText(bgCtx, channelCfg, fromPhone, reply); err != nil {
			s.logger.Warn("waWebhook: send reply failed",
				zap.String("provider", provider), zap.Error(err))
			return
		}
		// Persist the AI's reply as an outbound message so both sides of the
		// conversation show up in the inbox.
		if convID > 0 {
			_, _ = s.db.SaveWAMessage(convID, "outbound", reply, "text", "")
		}
	}()

	w.WriteHeader(http.StatusOK)
}
