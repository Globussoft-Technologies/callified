package realtime

import (
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

func TestBuildLiveSystemPromptAddsCampaignLanguageGuidance(t *testing.T) {
	got := buildLiveSystemPrompt(`Be helpful.
Respond only in Telugu.
ONLY switch language if the customer explicitly asks.
If they use another language, do NOT switch. Keep replying in the configured language.`, "te")

	assert.Contains(t, got, "Telugu (te)")
	assert.Contains(t, got, "controls ONLY the opening greeting")
	assert.Contains(t, got, "customer did not explicitly ask to switch")
	assert.Contains(t, got, "latest clearly recognized language")
	assert.Contains(t, got, "must not make you revert")
	assert.NotContains(t, got, "Respond only in Telugu")
	assert.NotContains(t, got, "ONLY switch language")
	assert.NotContains(t, got, "do NOT switch")
	assert.True(t, strings.HasSuffix(got, "Never announce or discuss a language switch."), "language policy must remain the final instruction")
}

func TestBuildLiveSystemPromptIgnoresUnknownLanguage(t *testing.T) {
	got := buildLiveSystemPrompt("Be helpful.", "unknown")

	assert.NotContains(t, got, "LIVE AUDIO LANGUAGE")
}
