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
	db        *db.DB
	llm       *llm.Provider
	ragClient *rag.Client
	log       *zap.Logger
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
	convID, err := a.db.GetOrCreateWAConversation(cfg.OrgID, msg.FromPhone, msg.Provider)
	if err != nil {
		return "", fmt.Errorf("GetOrCreateWAConversation: %w", err)
	}

	if msg.Text == "" {
		return "", nil // media-only message, no AI response
	}

	// Get recent chat history (last 10 messages)
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

	// Retrieve RAG context if available
	ragContext := ""
	if a.ragClient != nil {
		ragContext, _ = a.ragClient.RetrieveContext(ctx, msg.Text, 0, 3)
	}

	// Build system prompt from the configured product; fall back to generic.
	systemPrompt := a.buildSystemPrompt(cfg.DefaultProductID, ragContext)

	// Generate AI response
	reply, err := a.llm.GenerateResponse(ctx, systemPrompt, chatHistory, 200)
	if err != nil {
		a.log.Warn("wa agent: LLM failed", zap.Error(err))
		return "", nil
	}

	// Check for human takeover signal
	if strings.Contains(reply, humanTakeover) {
		reply = strings.ReplaceAll(reply, humanTakeover, "")
		reply = strings.TrimSpace(reply)
		a.log.Info("wa agent: human takeover triggered", zap.Int64("conv_id", convID))
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", nil
	}

	return reply, nil
}

// buildSystemPrompt constructs the system prompt for the AI. When a product
// is configured, its agent_persona, call_flow_instructions, manual_notes, and
// scraped_info are injected so the AI speaks as that product's sales agent.
// Falls back to a generic WhatsApp assistant prompt when no product is set.
func (a *Agent) buildSystemPrompt(productID int64, ragContext string) string {
	var prompt string

	if productID > 0 {
		product, err := a.db.GetProductByID(productID)
		if err == nil && product != nil {
			var parts []string

			if product.AgentPersona != "" {
				parts = append(parts, "## Agent Persona\n"+product.AgentPersona)
			}
			if product.CallFlowInstructions != "" {
				parts = append(parts, "## Instructions\n"+product.CallFlowInstructions)
			}
			if product.ManualNotes != "" {
				parts = append(parts, "## Product Notes\n"+product.ManualNotes)
			}
			if product.ScrapedInfo != "" {
				parts = append(parts, "## Product Information\n"+product.ScrapedInfo)
			}

			if len(parts) > 0 {
				prompt = "You are a WhatsApp sales assistant. Be concise and friendly — keep replies under 3 sentences unless the customer asks for details. Reply in the same language the customer uses.\n\n" +
					strings.Join(parts, "\n\n")
			}
		}
	}

	if prompt == "" {
		prompt = "You are a helpful WhatsApp sales assistant. Be concise, friendly, and professional. " +
			"Keep responses under 3 sentences unless the user asks a detailed question."
	}

	prompt += "\n\nIf you cannot help, offer to connect the customer with a human agent by saying " + humanTakeover + "."

	if ragContext != "" {
		prompt += "\n\n## Relevant Knowledge Base Context\n" + ragContext
	}

	return prompt
}
