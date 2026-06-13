package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ttsClient is reused across requests so connection pooling kicks in for the
// repeated round-trips to ElevenLabs over the course of a call.
var ttsClient = &http.Client{Timeout: 30 * time.Second}

type ttsRequest struct {
	Text    string `json:"text"`
	VoiceID string `json:"voice_id,omitempty"` // optional override per request
	Gender  string `json:"gender,omitempty"`   // "female" | "male" — picks default voice ID
}

// resolveVoiceID picks an ElevenLabs voice ID using this precedence:
//
//  1. Explicit voice_id in the request (per-call override)
//  2. Per-gender env var (ELEVENLABS_VOICE_ID_FEMALE / _MALE)
//  3. Generic ELEVENLABS_VOICE_ID (back-compat single-voice setup)
//
// Empty string means "no voice configured" — caller returns 503.
func resolveVoiceID(req ttsRequest) string {
	if req.VoiceID != "" {
		return req.VoiceID
	}
	switch strings.ToLower(req.Gender) {
	case "female":
		if v := os.Getenv("ELEVENLABS_VOICE_ID_FEMALE"); v != "" {
			return v
		}
	case "male":
		if v := os.Getenv("ELEVENLABS_VOICE_ID_MALE"); v != "" {
			return v
		}
	}
	return os.Getenv("ELEVENLABS_VOICE_ID")
}

// tts proxies POST /tts to ElevenLabs' text-to-speech endpoint and streams
// the resulting MP3 back to the browser. Voice and API key come from env vars
// (ELEVENLABS_API_KEY, ELEVENLABS_VOICE_ID); the request body may override the
// voice ID for one-off voice picks. We do NOT cache or persist audio — each
// utterance is fetched fresh so dynamic LLM replies render correctly.
func (s *Server) tts(w http.ResponseWriter, r *http.Request) {
	var req ttsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeErr(w, 400, "text is required")
		return
	}

	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		writeErr(w, 503, "ELEVENLABS_API_KEY not configured")
		return
	}
	voiceID := resolveVoiceID(req)
	if voiceID == "" {
		writeErr(w, 503, "no voice_id (set ELEVENLABS_VOICE_ID or _FEMALE/_MALE)")
		return
	}

	// ElevenLabs body: text + model + voice_settings. Model "eleven_turbo_v2_5"
	// is the low-latency multilingual model — ~400ms first-byte for short
	// utterances, suitable for a back-and-forth receptionist conversation.
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2_5",
		"voice_settings": map[string]any{
			"stability":         0.5,
			"similarity_boost":  0.75,
		},
	})

	req2, _ := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"https://api.elevenlabs.io/v1/text-to-speech/"+voiceID, strings.NewReader(string(body)))
	req2.Header.Set("xi-api-key", apiKey)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "audio/mpeg")

	resp, err := ttsClient.Do(req2)
	if err != nil {
		writeErr(w, 502, "elevenlabs request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// Surface the upstream error so the browser console shows it clearly
		// rather than playing silence.
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		writeErr(w, resp.StatusCode, "elevenlabs: "+string(buf))
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, resp.Body); err != nil && !errors.Is(err, io.EOF) {
		// Client likely disconnected mid-stream (e.g. user clicked New call);
		// not worth a 500.
		return
	}
}
