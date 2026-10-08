package recording

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/llm"
)

// Stereo input is halved before base64 encoding, keeping the inline request
// below Gemini's 20 MB limit even with the text prompt.
const maxVerificationAudioBytes = 16 * 1024 * 1024

type transcriptCorrection struct {
	Index      int    `json:"index"`
	Corrected  string `json:"corrected"`
	Confidence string `json:"confidence"`
}

type transcriptCorrectionResponse struct {
	Corrections []transcriptCorrection `json:"corrections"`
}

var transcriptDigits = regexp.MustCompile(`[0-9]+`)

// verifyCustomerTranscript listens to the customer channel after the call.
// Live audio and the conversation itself are never changed. Uncertain or
// malformed verification results leave the original transcript untouched.
func (s *Service) verifyCustomerTranscript(ctx context.Context, history []llm.ChatMessage, stereo []byte) ([]llm.ChatMessage, map[int]string, error) {
	if len(stereo) == 0 || len(stereo) > maxVerificationAudioBytes || s.cfg == nil {
		return history, nil, nil
	}
	apiKey := strings.TrimSpace(s.cfg.GeminiAPIKey)
	if apiKey == "" && strings.EqualFold(s.cfg.GeminiLiveAuthMode, "google_api_key") {
		apiKey = strings.TrimSpace(s.cfg.GeminiLiveAPIKey)
	}
	if apiKey == "" {
		return history, nil, nil
	}
	mono, err := audio.CustomerMonoWAV(stereo)
	if err != nil {
		return history, nil, err
	}
	userTexts := make([]string, 0)
	userHistoryIndices := make([]int, 0)
	for i, turn := range history {
		if strings.EqualFold(turn.Role, "user") && strings.TrimSpace(turn.Text) != "" {
			userHistoryIndices = append(userHistoryIndices, i)
			userTexts = append(userTexts, turn.Text)
		}
	}
	if len(userTexts) == 0 {
		return history, nil, nil
	}
	liveJSON, err := json.Marshal(userTexts)
	if err != nil {
		return history, nil, err
	}
	instruction := `Listen to the attached CUSTOMER-ONLY telephone audio. The following JSON array contains live customer transcription segments in chronological order: ` + string(liveJSON) + `.
Return ONLY JSON: {"corrections":[{"index":0,"corrected":"...","confidence":"high"}]}.
Index is zero-based into that array. Return only segments for which the audio CLEARLY proves a wrong word, especially place names, names, and addresses. Preserve the customer's language and exact meaning. Never infer words from sales context or an agent's reply; the agent channel is absent. Never guess unclear speech. Do not change numbers, dates, times, yes/no, or booking commitments. If none are certain, return {"corrections":[]}.`
	verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	client := llm.NewGeminiClient(apiKey, s.cfg.GeminiModel, s.cfg.GeminiBaseURL)
	result, err := client.GenerateAudioText(verifyCtx, instruction, mono, 2048)
	if err != nil {
		return history, nil, err
	}
	return applyTranscriptCorrections(history, userHistoryIndices, result)
}

func applyTranscriptCorrections(history []llm.ChatMessage, userHistoryIndices []int, result string) ([]llm.ChatMessage, map[int]string, error) {
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	var response transcriptCorrectionResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &response); err != nil {
		return history, nil, fmt.Errorf("invalid transcript verification JSON: %w", err)
	}
	verified := append([]llm.ChatMessage(nil), history...)
	originals := make(map[int]string)
	seen := make(map[int]bool)
	for _, correction := range response.Corrections {
		if correction.Index < 0 || correction.Index >= len(userHistoryIndices) || seen[correction.Index] || !strings.EqualFold(correction.Confidence, "high") {
			continue
		}
		seen[correction.Index] = true
		i := userHistoryIndices[correction.Index]
		original := strings.TrimSpace(verified[i].Text)
		corrected := strings.TrimSpace(correction.Corrected)
		if !safeTranscriptCorrection(original, corrected) {
			continue
		}
		originals[i] = original
		verified[i].Text = corrected
	}
	return verified, originals, nil
}

func safeTranscriptCorrection(original, corrected string) bool {
	if original == "" || corrected == "" || strings.EqualFold(original, corrected) || len(corrected) > len(original)*2+25 || len(corrected) > 300 {
		return false
	}
	// Never silently rewrite phone numbers, prices, appointment times, or a
	// short affirmative/negative. Those need a human/audio review.
	if strings.Join(transcriptDigits.FindAllString(original, -1), ",") != strings.Join(transcriptDigits.FindAllString(corrected, -1), ",") {
		return false
	}
	shortAnswer := map[string]bool{"yes": true, "no": true, "haan": true, "nahi": true, "okay": true, "ok": true}
	if shortAnswer[strings.ToLower(strings.Trim(original, " .!?"))] {
		return false
	}
	for _, r := range corrected {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
