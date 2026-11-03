package db

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreferredCustomerTranscriptText(t *testing.T) {
	raw := json.RawMessage(`[{"role":"User","text":"ಯಾವದು","suggested_text":"ಯಾವುದಕ್ಕಾಗಿ ಏನು?"},{"role":"AI","text":"Hello","suggested_text":"Ignored"},{"role":"User","text":"ಶಾಮ್","suggested_text":" "}]`)
	var turns []map[string]any
	require.NoError(t, json.Unmarshal(PreferredTranscriptText(raw), &turns))
	require.Equal(t, "ಯಾವುದಕ್ಕಾಗಿ ಏನು?", turns[0]["text"])
	require.Equal(t, "ಯಾವದು", turns[0]["original_text"])
	require.Equal(t, "audio_correction", turns[0]["text_source"])
	require.Equal(t, "Hello", turns[1]["text"])
	require.Equal(t, "ಶಾಮ್", turns[2]["text"])
	require.Contains(t, string(raw), `"text":"ಯಾವದು"`)
	encoded, err := json.Marshal(Transcript{ID: 1, Transcript: raw})
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"text":"ಯಾವುದಕ್ಕಾಗಿ ಏನು?"`)
}
