package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var ErrMaxTokens = errors.New("llm stopped at max output tokens")

// GeminiClient calls Google Gemini via REST SSE streaming.
type GeminiClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

func NewGeminiClient(apiKey, model, baseURL string) *GeminiClient {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &GeminiClient{apiKey: apiKey, baseURL: base, model: model, http: &http.Client{}}
}

// --- request types ---

type geminiRequest struct {
	SystemInstruction *geminiContent        `json:"system_instruction,omitempty"`
	Contents          []geminiContent       `json:"contents"`
	GenerationConfig  geminiStreamGenConfig `json:"generationConfig"`
	Tools             []geminiTool          `json:"tools,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiStreamGenConfig struct {
	MaxOutputTokens int32                 `json:"maxOutputTokens"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// --- response types (SSE) ---

type geminiStreamEvent struct {
	Candidates []struct {
		FinishReason string `json:"finishReason,omitempty"`
		Content      struct {
			Parts []struct {
				Text         string              `json:"text"`
				Thought      bool                `json:"thought,omitempty"`
				FunctionCall *geminiFunctionCall `json:"functionCall,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// geminiTextRequest is used for non-streaming generateContent calls.
// Supports thinkingConfig to disable reasoning for faster, complete responses.
type geminiTextRequest struct {
	SystemInstruction *geminiContent      `json:"system_instruction,omitempty"`
	Contents          []geminiContent     `json:"contents"`
	GenerationConfig  geminiTextGenConfig `json:"generationConfig"`
}

type geminiTextGenConfig struct {
	MaxOutputTokens int                   `json:"maxOutputTokens"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type geminiTextResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// GenerateText calls Gemini using the non-streaming REST endpoint with thinking disabled.
// Use this for batch/extract tasks (scraping, prompt generation) where streaming is not needed.
func (g *GeminiClient) GenerateText(ctx context.Context, systemPrompt, userMessage string, maxOutputTokens int) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("gemini: GEMINI_API_KEY not set")
	}

	body := geminiTextRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: userMessage}}},
		},
		GenerationConfig: geminiTextGenConfig{
			MaxOutputTokens: maxOutputTokens,
			ThinkingConfig:  &geminiThinkingConfig{ThinkingBudget: 0}, // disable thinking — not needed for extraction
		},
	}
	if systemPrompt != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal: %w", err)
	}

	endpoint, bearerAuth := g.endpoint("generateContent", false)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if bearerAuth {
		httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini: http: %w", err)
	}
	defer resp.Body.Close()

	var result geminiTextResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("gemini: decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("gemini: api error %d: %s", result.Error.Code, result.Error.Message)
	}
	var sb strings.Builder
	for _, cand := range result.Candidates {
		for _, part := range cand.Content.Parts {
			if part.Thought {
				continue
			}
			sb.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// StreamTokens streams tokens from Gemini, calling onToken for each text chunk.
// Uses SSE endpoint: streamGenerateContent?alt=sse
func (g *GeminiClient) StreamTokens(ctx context.Context, req TranscriptRequest, onToken func(string)) error {
	if g.apiKey == "" {
		return fmt.Errorf("gemini: GEMINI_API_KEY not set")
	}

	// Build contents: history + current user utterance
	contents := make([]geminiContent, 0, len(req.History)+1)
	for _, msg := range req.History {
		// Gemini's API rejects role="assistant" (used by OpenAI). Translate
		// the common synonym so callers built against the OpenAI shape don't
		// blow up here.
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Text}},
		})
	}
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: req.Transcript}},
	})

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: geminiStreamGenConfig{
			MaxOutputTokens: req.MaxTokens,
			// Voice turns need only the final spoken reply. Disabling thinking
			// on models that support zero prevents reasoning tokens from consuming
			// the realtime token budget. Other models still use the <SAY> gate.
			ThinkingConfig: voiceThinkingConfig(g.model),
		},
	}
	if req.SystemPrompt != "" {
		body.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}
	if req.EnableVoiceActions {
		body.Tools = voiceActionTools()
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gemini: marshal: %w", err)
	}

	endpoint, bearerAuth := g.endpoint("streamGenerateContent", true)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if bearerAuth {
		httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("gemini: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody) //nolint:errcheck
		return fmt.Errorf("gemini: status %d: %v", resp.StatusCode, errBody)
	}

	scanner := bufio.NewScanner(resp.Body)
	hitMaxTokens := false
	actionDelivered := false
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}
		var event geminiStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue // skip malformed chunk
		}
		if event.Error != nil {
			return fmt.Errorf("gemini: api error: %s", event.Error.Message)
		}
		for _, cand := range event.Candidates {
			if cand.FinishReason == "MAX_TOKENS" {
				hitMaxTokens = true
			}
			for _, part := range cand.Content.Parts {
				// Thought summaries are internal model output, even though the API
				// represents them as text parts. Never forward them to callers.
				if part.Thought {
					continue
				}
				if !actionDelivered && part.FunctionCall != nil && req.OnVoiceAction != nil {
					actionDelivered = true
					req.OnVoiceAction(VoiceAction{
						Name:       part.FunctionCall.Name,
						SpokenText: stringArg(part.FunctionCall.Args, "spoken_text"),
						Outcome:    stringArg(part.FunctionCall.Args, "outcome"),
					})
					continue
				}
				if part.Text != "" {
					onToken(part.Text)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if hitMaxTokens {
		return ErrMaxTokens
	}
	return nil
}

func voiceActionTools() []geminiTool {
	return []geminiTool{{
		FunctionDeclarations: []geminiFunctionDeclaration{{
			Name: "complete_call",
			Description: "Finish the phone call after the customer has explicitly confirmed a demo/appointment time, declined, or asked to end. " +
				"Do not call this while another question is needed.",
			Parameters: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"spoken_text": map[string]any{
						"type":        "STRING",
						"description": "One short customer-facing confirmation and goodbye in the required call language.",
					},
					"outcome": map[string]any{
						"type": "STRING",
						"enum": []string{"appointment_booked", "customer_declined", "customer_requested_end"},
					},
				},
				"required": []string{"spoken_text", "outcome"},
			},
		}},
	}}
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func voiceThinkingConfig(model string) *geminiThinkingConfig {
	// Gemini 2.5 Flash and Flash-Lite support thinkingBudget=0. Pro and newer
	// model families may require thinking, or use a different configuration;
	// omitting the field keeps those endpoints compatible.
	if strings.Contains(strings.ToLower(model), "gemini-2.5-flash") {
		return &geminiThinkingConfig{ThinkingBudget: 0}
	}
	return nil
}

func (g *GeminiClient) endpoint(method string, stream bool) (string, bool) {
	if g.baseURL != "" {
		endpoint := fmt.Sprintf("%s/v1beta/models/%s:%s", g.baseURL, url.PathEscape(g.model), method)
		if stream {
			endpoint += "?alt=sse"
		}
		return endpoint, true
	}

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:%s?key=%s",
		url.PathEscape(g.model), method, url.QueryEscape(g.apiKey),
	)
	if stream {
		endpoint += "&alt=sse"
	}
	return endpoint, false
}
