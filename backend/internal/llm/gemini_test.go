package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiGenerateTextUsesCompatibleThinkingConfig(t *testing.T) {
	for _, model := range []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.5-pro", "gemini-3.5-flash-lite"} {
		t.Run(model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				config := body["generationConfig"].(map[string]any)
				if model == "gemini-2.5-flash" || model == "gemini-2.5-flash-lite" {
					assert.Equal(t, map[string]any{"thinkingBudget": float64(0)}, config["thinkingConfig"])
				} else {
					assert.NotContains(t, config, "thinkingConfig")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Summary"}]}}]}`))
			}))
			defer server.Close()
			client := NewGeminiClient("test-key", model, server.URL)
			result, err := client.GenerateText(context.Background(), "Summarize", "Website text", 3000)
			require.NoError(t, err)
			assert.Equal(t, "Summary", result)
		})
	}
}

func TestGeminiStreamEventMarksThoughtParts(t *testing.T) {
	var event geminiStreamEvent
	err := json.Unmarshal([]byte(`{
		"candidates":[{"content":{"parts":[
			{"text":"internal reasoning","thought":true},
			{"text":"customer reply"}
		]}}]
	}`), &event)
	require.NoError(t, err)
	require.Len(t, event.Candidates, 1)
	require.Len(t, event.Candidates[0].Content.Parts, 2)
	assert.True(t, event.Candidates[0].Content.Parts[0].Thought)
	assert.False(t, event.Candidates[0].Content.Parts[1].Thought)
}

func TestGeminiDefaultEndpointUsesHeaderAPIKey(t *testing.T) {
	client := NewGeminiClient("test-key", "gemini-test", "")
	endpoint, bearerAuth := client.endpoint("generateContent", false)
	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	assert.Empty(t, parsed.Query().Get("key"))
	assert.False(t, bearerAuth)

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	require.NoError(t, err)
	client.applyAuth(req, bearerAuth)
	assert.Equal(t, "test-key", req.Header.Get("x-goog-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestGeminiCustomEndpointUsesBearerToken(t *testing.T) {
	client := NewGeminiClient("test-token", "gemini-test", "https://example.test")
	endpoint, bearerAuth := client.endpoint("streamGenerateContent", true)
	assert.True(t, bearerAuth)
	assert.Contains(t, endpoint, "?alt=sse")

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	require.NoError(t, err)
	client.applyAuth(req, bearerAuth)
	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-goog-api-key"))
}

func TestGeminiVoiceRequestDisablesThinking(t *testing.T) {
	thinking := voiceThinkingConfig("gemini-2.5-flash")
	require.NotNil(t, thinking)
	body := geminiRequest{
		GenerationConfig: geminiStreamGenConfig{
			MaxOutputTokens: 320,
			ThinkingConfig:  thinking,
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"contents":null,
		"generationConfig":{
			"maxOutputTokens":320,
			"thinkingConfig":{"thinkingBudget":0}
		}
	}`, string(raw))
}

func TestGeminiVoiceRequestKeepsOtherModelsCompatible(t *testing.T) {
	assert.Nil(t, voiceThinkingConfig("gemini-2.5-pro"))
	assert.Nil(t, voiceThinkingConfig("gemini-3-flash-preview"))
}

func TestGeminiStreamEventParsesCompleteCallFunction(t *testing.T) {
	var event geminiStreamEvent
	err := json.Unmarshal([]byte(`{
		"candidates":[{"content":{"parts":[{"functionCall":{
			"name":"complete_call",
			"args":{"spoken_text":"Thank you. Goodbye.","outcome":"appointment_booked"}
		}}]}}]
	}`), &event)
	require.NoError(t, err)
	call := event.Candidates[0].Content.Parts[0].FunctionCall
	require.NotNil(t, call)
	assert.Equal(t, "complete_call", call.Name)
	assert.Equal(t, "Thank you. Goodbye.", stringArg(call.Args, "spoken_text"))
}

func TestVoiceActionToolRequiresSpokenTextAndOutcome(t *testing.T) {
	raw, err := json.Marshal(voiceActionTools())
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"complete_call"`)
	assert.Contains(t, string(raw), `"spoken_text"`)
	assert.Contains(t, string(raw), `"appointment_booked"`)
}
