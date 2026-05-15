package wa

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/llm"
	"github.com/globussoft/callified-backend/internal/rag"
)

const humanTakeover = "[HUMAN_TAKEOVER]"
const pauseKey = "wa:pause:" // Redis key prefix for pause flag

// Agent processes inbound WhatsApp messages and generates AI replies.
type Agent struct {
	db       *db.DB
	llm      *llm.Provider
	ragClient *rag.Client
	log      *zap.Logger
}

// NewAgent creates a WA Agent.
func NewAgent(database *db.DB, llmProvider *llm.Provider, ragClient *rag.Client, log *zap.Logger) *Agent {
	return &Agent{
		db:        database,
		llm:       llmProvider,
		ragClient: ragClient,
		log:       log,
	}
}

// ProcessIncoming handles one inbound WA message and returns the reply text.
// Returns ("", nil) if no reply should be sent (e.g. human takeover active).
func (a *Agent) ProcessIncoming(ctx context.Context, cfg ChannelConfig, msg *IncomingMessage) (string, error) {
	// Dedup is handled by the webhook handler (see handleWAWebhook): it
	// saves the inbound message once before invoking the agent and skips
	// the agent path if a row with this provider_msg_id already exists.
	// The agent here only needs the conversation id to read history and
	// write the AI reply.
	convID, err := a.db.GetOrCreateWAConversation(cfg.OrgID, msg.FromPhone, msg.Provider)
	if err != nil {
		return "", fmt.Errorf("GetOrCreateWAConversation: %w", err)
	}

	if msg.Text == "" {
		return "", nil // media-only message, no AI response
	}

	// 4. Get recent chat history (last 10 messages)
	history, _ := a.db.GetWAChatHistory(convID, 10)
	var chatHistory []llm.ChatMessage
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		role := "user"
		if h.Direction == "outbound" {
			role = "model" // Gemini expects "model", not "assistant"
		}
		chatHistory = append(chatHistory, llm.ChatMessage{Role: role, Text: h.MessageText})
	}

	// 5. Retrieve RAG context if available
	ragContext := ""
	if a.ragClient != nil {
		ragContext, _ = a.ragClient.RetrieveContext(ctx, msg.Text, 0, 3)
	}

	// 6. Build system prompt
	systemPrompt := buildWASystemPrompt(ragContext)

	// 7. Generate AI response
	reply, err := a.llm.GenerateResponse(ctx, systemPrompt, chatHistory, 200)
	if err != nil {
		a.log.Warn("wa agent: LLM failed", zap.Error(err))
		return "", nil
	}

	// 8. Check for human takeover signal
	if strings.Contains(reply, humanTakeover) {
		reply = strings.ReplaceAll(reply, humanTakeover, "")
		reply = strings.TrimSpace(reply)
		a.log.Info("wa agent: human takeover triggered", zap.Int64("conv_id", convID))
	}

	reply = strings.TrimSpace(reply)
	// Fallback: if the LLM returned only the takeover marker (which we
	// stripped above), still send a friendly default so the customer
	// gets *something* on WhatsApp instead of a silent drop. The takeover
	// itself is already logged for operator follow-up.
	if reply == "" {
		reply = "Hi! Thanks for reaching out. I've notified a team member who'll get back to you shortly."
	}

	// Note: the OUTBOUND row is saved by the caller (webhook handler) only
	// AFTER the provider actually accepts the send. Saving it here meant
	// a WaSender 200-but-success:false (session disconnected, invalid JID)
	// left a phantom bubble in the inbox for a message that never went
	// out — confusing the operator.
	_ = convID

	return reply, nil
}

func buildWASystemPrompt(ragContext string) string {
	// Always answer greetings/small-talk yourself — do NOT escalate to a
	// human for "hi", "hello", or simple questions. Earlier prompt was too
	// eager: every greeting came back as just "[HUMAN_TAKEOVER]" which
	// produced an empty reply and the customer thought the bot was dead.
	base := `You are a helpful WhatsApp sales assistant. Be concise, friendly, and professional.
Keep responses under 3 sentences unless the user asks a detailed question.
Always greet back and try to help yourself first.
Only include ` + humanTakeover + ` (as the LAST line, after your normal reply) if the user
explicitly asks for a human agent OR you genuinely cannot answer their question.
Never reply with just ` + humanTakeover + ` alone — you must always send a friendly text reply too.`

	if ragContext != "" {
		base += "\n\nRelevant context from knowledge base:\n" + ragContext
	}
	return base
}
