package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
