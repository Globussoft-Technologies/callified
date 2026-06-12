package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

	// HMAC-SHA256 signature verification. Meta sends X-Hub-Signature-256
	// as "sha256=<hex>". Skip only when app secret is not configured (dev mode).
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	if s.cfg.MetaAppSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyMetaSignature(body, sig, s.cfg.MetaAppSecret) {
			s.logger.Sugar().Warnw("waWebhookMeta: signature mismatch", "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	s.handleWAWebhookBody(w, r, body, "meta", wa.ParseMeta)
}

func verifyMetaSignature(body []byte, header, secret string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

// handleWAWebhook is the shared handler for all inbound WA provider webhooks.
func (s *Server) handleWAWebhook(w http.ResponseWriter, r *http.Request, provider string,
	parser func([]byte) (*wa.IncomingMessage, error)) {

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.handleWAWebhookBody(w, r, body, provider, parser)
}

// handleWAWebhookBody processes an already-read webhook body. Used by providers
// (e.g. Meta) that need to read the body early for signature verification.
func (s *Server) handleWAWebhookBody(w http.ResponseWriter, r *http.Request, body []byte, provider string,
	parser func([]byte) (*wa.IncomingMessage, error)) {

	msg, err := parser(body)
	if err != nil || msg == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Look up the channel config by provider + destination phone.
	cfg, _ := s.db.GetWAChannelConfigByPhone(provider, msg.ToPhone)
	if cfg == nil {
		cfg, _ = s.db.GetSingleActiveWAChannelConfig(provider)
	}
	// For Meta: if no DB config exists, fall back to platform env credentials
	// so the webhook works without any UI configuration.
	var orgID int64
	if cfg != nil {
		orgID = cfg.OrgID
	} else if provider == "meta" && s.cfg.MetaAccessToken != "" {
		orgID = s.cfg.MetaDefaultOrgID
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

	// Build effective channel config — prefer DB config, fall back to env for Meta.
	var effectiveCfg wa.ChannelConfig
	if cfg != nil {
		if !cfg.AIEnabled {
			w.WriteHeader(http.StatusOK)
			return
		}
		if s.db.IsWAConversationMuted(cfg.OrgID, fromPhone) {
			w.WriteHeader(http.StatusOK)
			return
		}
		effectiveCfg = s.waChannelConfig(cfg.OrgID, cfg.Provider, cfg.PhoneNumber, cfg.APIKey, cfg.AppID, cfg.DefaultProductID)
	} else if provider == "meta" && s.cfg.MetaAccessToken != "" {
		effectiveCfg = s.waChannelConfig(orgID, "meta", "", "", "", 0)
	} else {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process with AI agent (async so we return 200 quickly).
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go func() {
		defer cancel()
		if s.waAgent == nil {
			return
		}
		channelCfg := effectiveCfg
		reply, imageURLs, err := s.waAgent.ProcessIncoming(bgCtx, channelCfg, msg)
		if err != nil {
			s.logger.Warn("waWebhook: agent failed",
				zap.String("provider", provider), zap.Error(err))
			return
		}
		s.logger.Info("waWebhook: agent result",
			zap.String("provider", provider),
			zap.String("from", fromPhone),
			zap.Int("reply_len", len(reply)),
			zap.Int("image_count", len(imageURLs)),
			zap.Strings("image_urls", imageURLs))
		if reply == "" && len(imageURLs) == 0 {
			return
		}
		// Send text reply first (if any).
		if reply != "" {
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
		}
		// Send product images after text.
		for _, imgURL := range imageURLs {
			if err := wa.SendImage(bgCtx, channelCfg, fromPhone, imgURL); err != nil {
				s.logger.Warn("waWebhook: send image failed",
					zap.String("provider", provider),
					zap.String("imgURL", imgURL),
					zap.Error(err))
			} else if convID > 0 {
				// Persist each sent image so the inbox can display it.
				_, _ = s.db.SaveWAMessage(convID, "outbound", imgURL, "image", "")
			}
		}
	}()

	w.WriteHeader(http.StatusOK)
}
