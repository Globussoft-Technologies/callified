package api

import (
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/wa"
)

// POST /wa/webhook/gupshup
func (s *Server) waWebhookGupshup(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "gupshup", wa.ParseGupshup)
}

// POST /wa/webhook/wati
func (s *Server) waWebhookWati(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "wati", wa.ParseWati)
}

// POST /wa/webhook/aisensei
func (s *Server) waWebhookAiSensei(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "aisensei", wa.ParseAiSensei)
}

// POST /wa/webhook/interakt
func (s *Server) waWebhookInterakt(w http.ResponseWriter, r *http.Request) {
	s.handleWAWebhook(w, r, "interakt", wa.ParseInterakt)
}

// POST /wa/webhook/meta — inbound messages
// GET  /wa/webhook/meta — Meta hub.challenge verification
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
	var orgID int64
	if cfg != nil {
		orgID = cfg.OrgID
	}

	// Always persist the inbound message before any AI processing so the
	// Inbox shows it. Previously the AI branch skipped the save entirely,
	// and the non-AI branch saved with org_id=0 which orphaned the row
	// (every org's /api/wa/conversations filters by the authed user's
	// org_id, so org_id=0 rows are invisible to everyone).
	convID, _ := s.db.GetOrCreateWAConversation(orgID, msg.FromPhone, provider)
	if convID > 0 {
		_, _ = s.db.SaveWAMessage(convID, "inbound", msg.Text, msg.MessageType, msg.ProviderMsgID)
	}

	if cfg == nil || !cfg.AIEnabled {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process with AI agent (async so we return 200 quickly)
	go func() {
		if s.waAgent == nil {
			return
		}
		channelCfg := s.waChannelConfig(cfg.Provider, cfg.PhoneNumber, cfg.APIKey, cfg.AppID)
		reply, err := s.waAgent.ProcessIncoming(r.Context(), channelCfg, msg)
		if err != nil {
			s.logger.Warn("waWebhook: agent failed",
				zap.String("provider", provider), zap.Error(err))
			return
		}
		if reply == "" {
			return
		}
		if err := s.waSender.SendText(r.Context(), channelCfg, msg.FromPhone, reply); err != nil {
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
