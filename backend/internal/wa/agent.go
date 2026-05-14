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

	// Get recent chat history (last 4 messages = ~2 exchanges). Keeping this
	// short prevents old AI messages — generated before a product was configured
	// — from polluting the context and causing the model to hallucinate the
	// wrong product category.
	history, _ := a.db.GetWAChatHistory(convID, 4)
	var chatHistory []llm.ChatMessage
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		role := "user"
		if h.Direction == "outbound" {
			role = "model" // Gemini expects "model", not "assistant"
		}
		chatHistory = append(chatHistory, llm.ChatMessage{Role: role, Text: h.MessageText})
	}

	// Retrieve RAG context scoped to this org.
	ragContext := ""
	if a.ragClient != nil {
		ragContext, _ = a.ragClient.RetrieveContext(ctx, msg.Text, cfg.OrgID, 3)
	}

	// Build system prompt from the configured product; fall back to generic.
	systemPrompt := a.buildSystemPrompt(cfg.DefaultProductID, ragContext)

	// Generate AI response
	reply, err := a.llm.GenerateResponse(ctx, systemPrompt, chatHistory, 300)
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
// is configured, the product's agent_persona becomes the AI's core identity,
// its scraped_info and manual_notes provide product knowledge, and
// call_flow_instructions guide the conversation. Falls back to a generic
// WhatsApp assistant when no product is set.
func (a *Agent) buildSystemPrompt(productID int64, ragContext string) string {
	const chatRules = "Be concise and friendly — keep replies under 3 sentences unless the customer asks for details. Reply in the same language the customer uses. Do not mention call scripts or phone steps; this is a WhatsApp chat."

	var prompt string

	if productID > 0 {
		product, err := a.db.GetProductByID(productID)
		if err == nil && product != nil {
			var parts []string

			// Hard constraint first so Gemini cannot hallucinate unrelated
			// products, even when the conversation history contains prior AI
			// messages that mentioned the wrong products. Those prior messages
			// were generated before the product was configured and must be
			// disregarded completely.
			parts = append(parts, "CRITICAL INSTRUCTION: You are exclusively a representative of "+product.Name+
				". Disregard ANY prior AI messages in this conversation that mention products, "+
				"categories, or services not related to "+product.Name+
				" — those were sent in error. Never claim to sell electronics, home goods, "+
				"apparel, or anything not described in the product information below. "+
				"If asked about unrelated products, clearly state that you only represent "+product.Name+".")

			// Agent persona is the AI's identity after the constraint.
			if product.AgentPersona != "" {
				parts = append(parts, product.AgentPersona)
			}

			// Product knowledge the AI can draw on to answer questions.
			if product.ScrapedInfo != "" {
				parts = append(parts, "## About "+product.Name+"\n"+product.ScrapedInfo)
			}
			if product.ManualNotes != "" {
				parts = append(parts, "## Additional Notes\n"+product.ManualNotes)
			}

			// Conversation guide (call-flow adapted for chat).
			if product.CallFlowInstructions != "" {
				parts = append(parts, "## Conversation Guide\n"+product.CallFlowInstructions)
			}

			prompt = strings.Join(parts, "\n\n") + "\n\n" + chatRules
		}
	}

	if prompt == "" {
		prompt = "You are a helpful WhatsApp sales assistant. " + chatRules
	}

	prompt += "\n\nIf you cannot help the customer, offer to connect them with a human agent by saying " + humanTakeover + "."

	if ragContext != "" {
		prompt += "\n\n## Relevant Knowledge Base\n" + ragContext
	}

	return prompt
}
