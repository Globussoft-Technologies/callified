package realtime

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupEnablesNativeLiveAudioFeatures(t *testing.T) {
	client := New(Config{
		Model: "gemini-3.8-live", Voice: "Kore", Language: "te",
		SystemPrompt: "Product: Globus CRM",
	}, Callbacks{})
	raw, err := json.Marshal(client.setupMessage())
	assert.NoError(t, err)
	setup := string(raw)

	assert.Contains(t, setup, `"responseModalities":["AUDIO"]`)
	assert.Contains(t, setup, `"voiceName":"Kore"`)
	assert.Contains(t, setup, `"inputAudioTranscription":{}`)
	assert.Contains(t, setup, `"outputAudioTranscription":{}`)
	assert.Contains(t, setup, `"activityHandling":"START_OF_ACTIVITY_INTERRUPTS"`)
	assert.Contains(t, setup, `"startOfSpeechSensitivity":"START_SENSITIVITY_HIGH"`)
	assert.Contains(t, setup, `"turnCoverage":"TURN_INCLUDES_ONLY_ACTIVITY"`)
	assert.Contains(t, setup, `"silenceDurationMs":600`)
	assert.Contains(t, setup, `"contextWindowCompression":{"slidingWindow":{}}`)
	assert.Contains(t, setup, `"sessionResumption":{}`)
	assert.Contains(t, setup, `"complete_call"`)
	assert.Contains(t, setup, `"search_product_knowledge"`)
	assert.Contains(t, setup, `"appointment_date"`)
	assert.Contains(t, setup, `"appointment_time"`)
	assert.Contains(t, setup, "Product: Globus CRM")
}

func TestBuildLiveSystemPromptRemovesLegacyHangupInstructions(t *testing.T) {
	prompt := `You are a helpful sales agent.
End the final reply with [HANGUP].
Keep answers short.
Example: Thank you. [hangup]`

	got := buildLiveSystemPrompt(prompt, "te")

	assert.NotContains(t, strings.ToUpper(got), "[HANGUP]")
	assert.Contains(t, got, "You are a helpful sales agent.")
	assert.Contains(t, got, "Keep answers short.")
	assert.Contains(t, got, "call complete_call")
	assert.Contains(t, got, "same unanswered CALL FLOW step")
	assert.Contains(t, got, "Never reveal or discuss Gemini")
}

func TestRequestResponseUsesBoundedLiveControlQueue(t *testing.T) {
	client := New(Config{}, Callbacks{})
	assert.True(t, client.RequestResponse("close the call"))
	assert.False(t, client.RequestResponse("   "))
}

func TestRejectedAppointmentToolResponseRequiresNewCustomerConfirmation(t *testing.T) {
	got := rejectedCompleteCallResult("appointment_booked")
	assert.Contains(t, got, "customer has not personally confirmed both the day/date and exact time")
	assert.Contains(t, got, "Do not retry complete_call until the customer gives a new answer")
	assert.Contains(t, got, "Speak one short question")
	assert.Contains(t, got, "do not say goodbye")
}

func TestGeminiLiveDirectGoogleConnectionSettings(t *testing.T) {
	const googleLiveURL = "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"
	client := New(Config{URL: googleLiveURL, APIKey: "google-key", AuthMode: "google_api_key"}, Callbacks{})
	endpoint, header, err := client.connectionSettings()
	assert.NoError(t, err)
	assert.Equal(t, googleLiveURL, endpoint)
	assert.Equal(t, "google-key", header.Get("x-goog-api-key"))
	assert.Empty(t, header.Get("Authorization"))
}

func TestGeminiLiveCentralGatewayConnectionSettings(t *testing.T) {
	client := New(Config{
		URL:      "wss://centralized-gemini.globussoft.com/nx/direct/",
		APIKey:   "gateway-key",
		AuthMode: "bearer",
	}, Callbacks{})
	endpoint, header, err := client.connectionSettings()
	assert.NoError(t, err)
	assert.Equal(t, "wss://centralized-gemini.globussoft.com/nx/direct/", endpoint)
	assert.Equal(t, "Bearer gateway-key", header.Get("Authorization"))
	assert.Empty(t, header.Get("x-goog-api-key"))
}

func TestGeminiLiveRejectsInvalidConnectionConfiguration(t *testing.T) {
	tests := []Config{
		{APIKey: "key", AuthMode: "bearer"},
		{URL: "https://example.com/live", APIKey: "key", AuthMode: "bearer"},
		{URL: "ws://example.com/live", APIKey: "key", AuthMode: "bearer"},
		{URL: "wss://example.com/live", APIKey: "key", AuthMode: "unknown"},
		{URL: "wss://example.com/live", AuthMode: "bearer"},
	}
	for _, cfg := range tests {
		_, _, err := New(cfg, Callbacks{}).connectionSettings()
		assert.Error(t, err)
	}
}

func TestLiveControlMessageInterruptsGeneration(t *testing.T) {
	message := liveControlMessage("RESPOND IN TELUGU")
	clientContent := message["clientContent"].(map[string]any)
	assert.Equal(t, true, clientContent["turnComplete"])
	turns := clientContent["turns"].([]any)
	turn := turns[0].(map[string]any)
	parts := turn["parts"].([]any)
	assert.Equal(t, "RESPOND IN TELUGU", parts[0].(map[string]string)["text"])
}

func TestRealtimeAudioMessagePreservesEightKHzPCM(t *testing.T) {
	pcm8k := []byte{0x01, 0x02, 0x7f, 0x80, 0xfe, 0xff}
	msg := realtimeAudioMessage(pcm8k)
	realtimeInput := msg["realtimeInput"].(map[string]any)
	audioPart := realtimeInput["audio"].(map[string]string)

	assert.Equal(t, "audio/pcm;rate=8000", audioPart["mimeType"])
	decoded, err := base64.StdEncoding.DecodeString(audioPart["data"])
	assert.NoError(t, err)
	assert.Equal(t, pcm8k, decoded)
}

func TestBuildLiveSystemPromptAddsCampaignLanguageGuidance(t *testing.T) {
	got := buildLiveSystemPrompt(`[LANG:te]
Be helpful.
Respond only in Telugu.
## LANGUAGE
- Banned formal/written register: example
- English words (e.g. meeting) mix in naturally.
ONLY switch language if the customer explicitly asks.
If they use another language, do NOT switch. Keep replying in the configured language.`, "te")

	assert.Contains(t, got, "Telugu (te)")
	assert.Contains(t, got, "controls ONLY the opening greeting")
	assert.Contains(t, got, "explicit language request")
	assert.Contains(t, got, "CURRENT_REPLY_LANGUAGE")
	assert.Contains(t, got, "unambiguous native script")
	assert.Contains(t, got, "one unclear short Latin-script phrase")
	assert.Contains(t, got, "ENGLISH VOICE AND ACCENT — HIGHEST PRIORITY")
	assert.Contains(t, got, "use ONLY a clear, natural Indian English accent")
	assert.Contains(t, got, "entire utterance from its first word to its last")
	assert.Contains(t, got, "Never drift into or imitate an American, British, Australian")
	assert.NotContains(t, got, "Respond only in Telugu")
	assert.NotContains(t, got, "ONLY switch language")
	assert.NotContains(t, got, "do NOT switch")
	assert.NotContains(t, got, "[LANG:te]")
	assert.NotContains(t, got, "## LANGUAGE")
	assert.NotContains(t, got, "Banned formal/written register")
	assert.NotContains(t, got, "English words (e.g.")
	assert.True(t, strings.HasSuffix(got, "Never announce or discuss a language switch."), "language policy must remain the final instruction")
}

func TestBuildLiveSystemPromptIgnoresUnknownLanguage(t *testing.T) {
	got := buildLiveSystemPrompt("Be helpful.", "unknown")

	assert.NotContains(t, got, "LIVE AUDIO LANGUAGE")
}
