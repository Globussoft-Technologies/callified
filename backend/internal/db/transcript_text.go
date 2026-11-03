package db

import (
	"encoding/json"
	"strings"
)

// PreferredTranscriptText presents available audio-review corrections while
// preserving the live recognizer's text. Stored transcripts remain unchanged.
func PreferredTranscriptText(raw json.RawMessage) json.RawMessage {
	var turns []map[string]any
	if json.Unmarshal(raw, &turns) != nil {
		return raw
	}
	changed := false
	for _, turn := range turns {
		role, _ := turn["role"].(string)
		if !strings.EqualFold(role, "user") && !strings.EqualFold(role, "customer") {
			continue
		}
		suggestion, _ := turn["suggested_text"].(string)
		if strings.TrimSpace(suggestion) == "" {
			continue
		}
		if _, exists := turn["original_text"]; !exists {
			turn["original_text"] = turn["text"]
		}
		turn["text"] = suggestion
		turn["text_source"] = "audio_correction"
		changed = true
	}
	if !changed {
		return raw
	}
	result, err := json.Marshal(turns)
	if err != nil {
		return raw
	}
	return result
}

func (t Transcript) MarshalJSON() ([]byte, error) {
	type plain Transcript
	copy := plain(t)
	copy.Transcript = PreferredTranscriptText(t.Transcript)
	return json.Marshal(copy)
}
