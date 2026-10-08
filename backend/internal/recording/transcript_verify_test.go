package recording

import (
	"encoding/json"
	"testing"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/llm"
)

func TestCustomerAudioCorrectionPreservesOriginalAndAgent(t *testing.T) {
	history := []llm.ChatMessage{{Role: "model", Text: "Where?"}, {Role: "user", Text: "code mongla"}, {Role: "model", Text: "Thanks."}}
	verified, suggestions, err := applyTranscriptCorrections(history, []int{1}, `{"corrections":[{"index":0,"corrected":"Koramangala","confidence":"high"}]}`)
	if err != nil || verified[1].Text != "code mongla" || suggestions[1] != "Koramangala" || verified[0].Text != "Where?" || history[1].Text != "code mongla" {
		t.Fatalf("unexpected correction: history=%v suggestions=%v err=%v", verified, suggestions, err)
	}
	encoded, _ := historyToTranscriptWithSuggestions(verified, suggestions)
	var turns []map[string]string
	if err := json.Unmarshal([]byte(encoded), &turns); err != nil || turns[1]["suggested_text"] != "Koramangala" || turns[1]["text"] != "code mongla" {
		t.Fatalf("persisted transcript lost audit text: %s err=%v", encoded, err)
	}
}

func TestCustomerAudioSuggestionsCannotRewriteAutomatedDecisions(t *testing.T) {
	for _, tc := range []struct{ original, corrected string }{
		{"I do not want an appointment", "I want an appointment"},
		{"Tomorrow at ten in the morning", "Tomorrow at eleven in the morning"},
		{"नहीं", "हाँ"},
	} {
		history := []llm.ChatMessage{{Role: "user", Text: tc.original}}
		response, _ := json.Marshal(transcriptCorrectionResponse{Corrections: []transcriptCorrection{{Index: 0, Corrected: tc.corrected, Confidence: "high"}}})
		canonical, suggestions, err := applyTranscriptCorrections(history, []int{0}, string(response))
		if err != nil || canonical[0].Text != tc.original || history[0].Text != tc.original {
			t.Fatalf("automated analysis history was rewritten: %v, err=%v", canonical, err)
		}
		encoded, _ := historyToTranscriptWithSuggestions(canonical, suggestions)
		var turns []map[string]string
		if err := json.Unmarshal([]byte(encoded), &turns); err != nil || turns[0]["text"] != tc.original || turns[0]["suggested_text"] != tc.corrected {
			t.Fatalf("canonical text or review suggestion lost: %s, err=%v", encoded, err)
		}
	}
}

func TestCustomerAudioCorrectionRejectsRiskyChanges(t *testing.T) {
	for _, tc := range []struct{ original, corrected string }{
		{"10 AM", "11 AM"}, {"Yes", "No"}, {"Koramangala", "A completely different long made-up appointment sentence"},
	} {
		if safeTranscriptCorrection(tc.original, tc.corrected) {
			t.Errorf("accepted risky correction %q -> %q", tc.original, tc.corrected)
		}
	}
}

func TestCustomerMonoWAVExcludesAgentChannel(t *testing.T) {
	stereo := audio.BuildStereoWAV([]audio.TimedChunk{{Data: []byte{1, 0, 2, 0}}}, nil)
	mono, err := audio.CustomerMonoWAV(stereo)
	if err != nil || len(mono) == 0 {
		t.Fatalf("customer channel extraction failed: %v", err)
	}
	if _, err := audio.CustomerMonoWAV([]byte("bad")); err == nil {
		t.Fatal("malformed WAV accepted")
	}
}
