package wa

import (
	"context"
	"fmt"
	"net/url"
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

// ProcessIncoming handles one inbound WA message and returns the reply text
// plus any image URLs the AI wants to send.
// Returns ("", nil, nil) if no reply should be sent (e.g. human takeover active).
func (a *Agent) ProcessIncoming(ctx context.Context, cfg ChannelConfig, msg *IncomingMessage) (reply string, imageURLs []string, err error) {
	convID, err := a.db.GetOrCreateWAConversation(cfg.OrgID, msg.FromPhone, msg.Provider)
	if err != nil {
		return "", nil, fmt.Errorf("GetOrCreateWAConversation: %w", err)
	}

	if msg.Text == "" {
		return "", nil, nil // media-only message, no AI response
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

	// Campaign-based product overrides the channel default: if this phone number
	// belongs to a lead in a WhatsApp campaign, use that campaign's product.
	effectiveProductID := cfg.DefaultProductID
	if campaign, err := a.db.GetActiveCampaignForLeadPhone(cfg.OrgID, msg.FromPhone); err == nil && campaign != nil && campaign.ProductID > 0 {
		effectiveProductID = campaign.ProductID
	}

	// Build system prompt from the effective product; fall back to generic.
	systemPrompt := a.buildSystemPrompt(effectiveProductID, ragContext)

	// Generate AI response
	raw, err := a.llm.GenerateResponse(ctx, systemPrompt, chatHistory, 300)
	if err != nil {
		a.log.Warn("wa agent: LLM failed", zap.Error(err))
		return "", nil, nil
	}

	// Check for human takeover signal
	if strings.Contains(raw, humanTakeover) {
		raw = strings.ReplaceAll(raw, humanTakeover, "")
		raw = strings.TrimSpace(raw)
		a.log.Info("wa agent: human takeover triggered", zap.Int64("conv_id", convID))
	}

	// Parse [SEND_IMAGE:url] tokens from the reply; strip them from the text.
	imageURLs, raw = parseImageSignals(raw)

	reply = strings.TrimSpace(raw)
	if reply == "" && len(imageURLs) == 0 {
		return "", nil, nil
	}

	return reply, imageURLs, nil
}

// imageLabel derives a human-readable label from an image URL by extracting
// and cleaning the filename. E.g. ".../Morais-Lavender.avif" → "Morais Lavender".
func imageLabel(rawURL string) string {
	// Take last path segment
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 {
		rawURL = rawURL[idx+1:]
	}
	// Strip query string
	if q := strings.IndexByte(rawURL, '?'); q >= 0 {
		rawURL = rawURL[:q]
	}
	// URL-decode percent-encoded characters (e.g. OFFICE%20SPACE → OFFICE SPACE)
	if decoded, err := url.PathUnescape(rawURL); err == nil {
		rawURL = decoded
	}
	// Strip extension
	if dot := strings.LastIndex(rawURL, "."); dot >= 0 {
		rawURL = rawURL[:dot]
	}
	// Replace separators with spaces
	rawURL = strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(rawURL)
	words := strings.Fields(rawURL)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// parseImageSignals extracts [SEND_IMAGE:url] tokens from text, returning the
// list of URLs and the cleaned text with the tokens removed.
func parseImageSignals(text string) ([]string, string) {
	const prefix = "[SEND_IMAGE:"
	const suffix = "]"

	var urls []string
	var cleaned strings.Builder

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && strings.HasSuffix(trimmed, suffix) {
			// Extract the URL from [SEND_IMAGE:url]
			inner := trimmed[len(prefix) : len(trimmed)-len(suffix)]
			inner = strings.TrimSpace(inner)
			if strings.HasPrefix(inner, "http") && len(urls) < 3 {
				urls = append(urls, inner)
			}
			continue
		}
		// Check for inline [SEND_IMAGE:...] within a line
		remaining := line
		for {
			start := strings.Index(remaining, prefix)
			if start < 0 {
				cleaned.WriteString(remaining)
				break
			}
			end := strings.Index(remaining[start:], suffix)
			if end < 0 {
				cleaned.WriteString(remaining)
				break
			}
			cleaned.WriteString(remaining[:start])
			token := remaining[start : start+end+len(suffix)]
			inner := token[len(prefix) : len(token)-len(suffix)]
			inner = strings.TrimSpace(inner)
			if strings.HasPrefix(inner, "http") && len(urls) < 3 {
				urls = append(urls, inner)
			}
			remaining = remaining[start+end+len(suffix):]
		}
		cleaned.WriteString("\n")
	}

	return urls, strings.TrimSpace(cleaned.String())
}

// buildSystemPrompt constructs the system prompt for the AI. When a product
// is configured, the product's agent_persona becomes the AI's core identity,
// its scraped_info and manual_notes provide product knowledge, and
// call_flow_instructions guide the conversation. Falls back to a generic
// WhatsApp assistant when no product is set.
func (a *Agent) buildSystemPrompt(productID int64, ragContext string) string {
	const chatRules = "Be concise and friendly — keep replies under 3 sentences unless the customer asks for details. IMPORTANT: Reply in English by default. Only switch to another language if the customer's CURRENT message is clearly written in that language — do NOT match the language of previous AI messages in the history. Do not mention call scripts or phone steps; this is a WhatsApp chat."

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

			// Product images: manual uploads first (user-defined labels), then scraped.
			hasManual := len(product.ManualImages) > 0
			hasScraped := len(product.ImageURLs) > 0
			if hasManual || hasScraped {
				var imgSection strings.Builder
				imgSection.WriteString("## Product Images — Send the RIGHT image for each query\n\n")
				imgSection.WriteString("Each image below has a LABEL. When a customer asks about a specific item, feature, or property:\n")
				imgSection.WriteString("1. Match their query keyword to the closest LABEL.\n")
				imgSection.WriteString("2. Write one short intro line.\n")
				imgSection.WriteString("3. On the NEXT line output the matching token EXACTLY: [SEND_IMAGE:URL]\n")
				imgSection.WriteString("4. If they ask for 'images' / 'photos' in general (no specific item), send the first 2-3 tokens.\n")
				imgSection.WriteString("RULE: NEVER output a [SEND_IMAGE:] token with a URL not listed below.\n\n")
				imgSection.WriteString("LABEL → [SEND_IMAGE:URL]:\n")
				// Manual images take priority — labels are set by the user.
				for _, img := range product.ManualImages {
					imgSection.WriteString("- " + img.Label + " → [SEND_IMAGE:" + img.URL + "]\n")
				}
				// Scraped images follow.
				for _, u := range product.ImageURLs {
					label := imageLabel(u)
					imgSection.WriteString("- " + label + " → [SEND_IMAGE:" + u + "]\n")
				}
				// Concrete example using first available image
				var exLabel, exURL string
				if hasManual {
					exLabel = product.ManualImages[0].Label
					exURL = product.ManualImages[0].URL
				} else {
					exLabel = imageLabel(product.ImageURLs[0])
					exURL = product.ImageURLs[0]
				}
				imgSection.WriteString("\nExample — customer: \"show me " + exLabel + "\"\n")
				imgSection.WriteString("Your reply: \"Here is " + exLabel + ":\n[SEND_IMAGE:" + exURL + "]\"\n")
				parts = append(parts, imgSection.String())
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
