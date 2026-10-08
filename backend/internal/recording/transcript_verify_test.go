package recording

import (
	"encoding/json"
	"testing"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/llm"
)

func TestCustomerAudioCorrectionPreservesOriginalAndAgent(t *testing.T) {
	history := []llm.ChatMessage{{Role: "model", Text: "Where?"}, {Role: "user", Text: "code mongla"}, {Role: "model", Text: "Thanks."}}
	verified, originals, err := applyTranscriptCorrections(history, []int{1}, `{"corrections":[{"index":0,"corrected":"Koramangala","confidence":"high"}]}`)
	if err != nil || verified[1].Text != "Koramangala" || originals[1] != "code mongla" || verified[0].Text != "Where?" || history[1].Text != "code mongla" {
		t.Fatalf("unexpected correction: history=%v originals=%v err=%v", verified, originals, err)
	}
	encoded, _ := historyToTranscriptWithOriginals(verified, originals)
	var turns []map[string]string
	if err := json.Unmarshal([]byte(encoded), &turns); err != nil || turns[1]["live_text"] != "code mongla" || turns[1]["text"] != "Koramangala" {
		t.Fatalf("persisted transcript lost audit text: %s err=%v", encoded, err)
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
