package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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

// ttsResult is the synth output handed back to the HTTP layer — bytes
// plus the MIME type so the response Content-Type matches reality
// (audio/mpeg for ElevenLabs, audio/wav for Smallest).
type ttsResult struct {
	body []byte
	mime string
}

// errQuota signals the upstream rejected the request because of an
// account/credit issue. Triggers fallback to the next provider in tts().
var errQuota = errors.New("tts: upstream quota exceeded")

// tts: POST /tts proxies to ElevenLabs first, then falls back to
// SmallestAI if ElevenLabs is unconfigured or out of credits. Without
// the fallback, callers hear browser speechSynthesis (which can't be
// recorded by WebAudio), so bot voice would not appear in the saved
// recording. Fallback keeps the bot voice in a real audio stream that
// the browser tap captures end-to-end.
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

	// Provider order:
	//   1. ElevenLabs if API key + voice_id are configured.
	//   2. Smallest if SMALLEST_API_KEY is set.
	// On quota failure from #1, drop to #2 silently — the user keeps
	// getting audio (no client-side speechSynthesis fallback), so the
	// browser's WebAudio tap captures bot voice into the recording.
	var lastErr error
	ctx := r.Context()
	if elevenAvailable(req) {
		out, err := synthElevenLabs(ctx, text, resolveVoiceID(req))
		if err == nil {
			writeAudio(w, out)
			return
		}
		lastErr = err
		// Only fall back on quota — every other error (network, 5xx) is
		// likely transient and the client should know about it. But for
		// quota_exceeded specifically, ElevenLabs is reliably broken
		// until billing is fixed, so silently switching to Smallest
		// keeps the demo working.
		if !errors.Is(err, errQuota) {
			writeErr(w, 502, "elevenlabs: "+err.Error())
			return
		}
	}

	if os.Getenv("SMALLEST_API_KEY") != "" {
		out, err := synthSmallest(ctx, text, req.Gender)
		if err == nil {
			writeAudio(w, out)
			return
		}
		lastErr = err
	}

	if lastErr != nil {
		writeErr(w, 502, "tts: "+lastErr.Error())
		return
	}
	writeErr(w, 503, "tts: no provider configured (set ELEVENLABS_API_KEY or SMALLEST_API_KEY)")
}

func writeAudio(w http.ResponseWriter, out *ttsResult) {
	w.Header().Set("Content-Type", out.mime)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(out.body); err != nil && !errors.Is(err, io.EOF) {
		return
	}
}

// --- ElevenLabs -------------------------------------------------------

func elevenAvailable(req ttsRequest) bool {
	return os.Getenv("ELEVENLABS_API_KEY") != "" && resolveVoiceID(req) != ""
}

// synthElevenLabs returns errQuota when the upstream rejects the request
// because of credits/billing, so the caller can fall back to Smallest.
// Other errors propagate as-is.
func synthElevenLabs(ctx context.Context, text, voiceID string) (*ttsResult, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2_5",
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.elevenlabs.io/v1/text-to-speech/"+voiceID,
		strings.NewReader(string(body)))
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := ttsClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		// quota_exceeded is reported as 401 with a JSON body containing
		// {"detail":{"status":"quota_exceeded",...}}. We also fall back
		// on 402 (paid_plan_required), which fires for "library voices"
		// the account doesn't own. Both are durable upstream conditions
		// that no retry will fix.
		s := string(buf)
		if resp.StatusCode == 401 || resp.StatusCode == 402 {
			if strings.Contains(s, "quota_exceeded") ||
				strings.Contains(s, "paid_plan_required") ||
				strings.Contains(s, "credits") {
				return nil, fmt.Errorf("%w: %s", errQuota, s)
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, s)
	}
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &ttsResult{body: buf, mime: "audio/mpeg"}, nil
}

// --- Smallest fallback ------------------------------------------------

// Smallest voice picks. The receptionist UI sends gender=female|male; we
// map that to two known Smallest voices. Override per env var if the
// account has different voices configured.
func smallestVoice(gender string) string {
	switch strings.ToLower(gender) {
	case "male":
		if v := os.Getenv("SMALLEST_VOICE_ID_MALE"); v != "" {
			return v
		}
		return "raj"
	default:
		if v := os.Getenv("SMALLEST_VOICE_ID_FEMALE"); v != "" {
			return v
		}
		if v := os.Getenv("SMALLEST_VOICE_ID"); v != "" {
			return v
		}
		return "emily"
	}
}

// synthSmallest calls SmallestAI's lightning endpoint and wraps the raw
// 8kHz/16-bit PCM response in a WAV header so the browser <audio>
// element can play it natively. WAV is uncompressed but at 8kHz mono
// the size is fine for a few-second utterance (~16 KB/s).
func synthSmallest(ctx context.Context, text, gender string) (*ttsResult, error) {
	apiKey := os.Getenv("SMALLEST_API_KEY")
	body, _ := json.Marshal(map[string]any{
		"text":           text,
		"voice_id":       smallestVoice(gender),
		"language":       "en",
		"sample_rate":    8000,
		"add_wav_header": false,
		"speed":          1.0,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://waves-api.smallest.ai/api/v1/lightning/get_speech",
		strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ttsClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("smallest HTTP %d: %s", resp.StatusCode, string(buf))
	}
	pcm, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	wav := wrapPCMAsWAV(pcm, 8000, 1, 16)
	return &ttsResult{body: wav, mime: "audio/wav"}, nil
}

// wrapPCMAsWAV emits a minimal RIFF/WAVE header followed by the raw PCM
// samples. Format-tag 1 = uncompressed PCM; channels/sampleRate/bitDepth
// describe the sample stream. Browsers play this natively.
func wrapPCMAsWAV(data []byte, sampleRate, channels, bitDepth int) []byte {
	var buf bytes.Buffer
	dataLen := len(data)
	blockAlign := channels * bitDepth / 8
	byteRate := sampleRate * blockAlign

	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(bitDepth))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	buf.Write(data)
	return buf.Bytes()
}
