package wsphone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/globussoft/callified-backend/internal/audio"
)

// outboundFrameBytes is one frame of µ-law audio at 8 kHz × 20ms.
// Exotel's media frames are this size; we match it so playback timing
// is consistent with what the carrier expects.
const outboundFrameBytes = 160

// httpClient is reused across calls so connection pooling kicks in for
// the repeated round-trips to ElevenLabs / Smallest during a long call.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// speak synthesizes text via ElevenLabs (or Smallest fallback) and
// streams the result back to the carrier as a sequence of "media"
// frames, each ~160 bytes µ-law. After all frames are sent, a "mark"
// event is emitted with name=markName so the WS loop can correlate
// "playback done" notifications.
//
// This call BLOCKS until all audio bytes have been queued to the WS.
// Pacing is wallclock — we sleep 20 ms between frames so the carrier's
// jitter buffer doesn't gulp the whole utterance and ignore barge-in.
// (See the audit's Critical #1: bursting all chunks at once breaks
// stereo recording AND stops barge-in from working.)
//
// Returns the number of bytes synthesized (0 on failure). Errors are
// logged, not returned, because the call should continue even if one
// utterance fails to synthesize — better silence on one turn than the
// whole call dying.
func (s *phoneSession) speak(ctx context.Context, text, markName string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if s.streamSid == "" {
		log.Printf("wsphone: speak called before start — dropping %q", text)
		return 0
	}

	// Mark TTS active so any inbound transcripts during playback can
	// be filtered. Reset on return so the next turn of inbound speech
	// is processed normally.
	atomic.StoreInt32(&s.ttsActive, 1)
	defer atomic.StoreInt32(&s.ttsActive, 0)

	// Per-utterance cancellable ctx so barge-in can interrupt this
	// speak() without affecting the call's outer ctx. Register the
	// cancel-fn on the session so cancelActiveTTS can find it.
	utterCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.setTTSCancel(cancel)
	defer s.clearTTSCancel()
	ctx = utterCtx

	pcm, err := s.synthesize(ctx, text)
	if err != nil {
		log.Printf("wsphone: synth failed for %s: %v", s.streamSid, err)
		return 0
	}
	if len(pcm) == 0 {
		return 0
	}

	// Capture bot PCM into the recording before we frame/encode it for
	// the carrier. Recorded as linear PCM so we can pair it with caller
	// PCM in the stereo WAV without sample-rate or codec conversion.
	s.appendBotPCM(pcm)

	// Convert PCM (linear 16-bit, 8kHz) → µ-law and slice into frames.
	// PCMToUlaw is from internal/audio/codec.go and matches what the
	// campaign pipeline uses, so we're not re-implementing codec logic.
	ulaw := audio.PCMToUlaw(pcm)
	totalBytes := 0
	for off := 0; off < len(ulaw); off += outboundFrameBytes {
		end := off + outboundFrameBytes
		if end > len(ulaw) {
			end = len(ulaw)
		}
		frame, err := encodeMediaFrame(s.streamSid, ulaw[off:end])
		if err != nil {
			log.Printf("wsphone: encodeMediaFrame: %v", err)
			return totalBytes
		}
		if err := s.writeFrame(frame); err != nil {
			log.Printf("wsphone: writeFrame: %v", err)
			return totalBytes
		}
		totalBytes += end - off

		// 20 ms pace per frame. Use ctx-aware sleep so a hangup mid-
		// utterance returns immediately rather than waiting out the
		// remaining buffered audio. This is the single biggest cause
		// of "bot keeps talking after caller hangs up" in carrier-side
		// integrations.
		select {
		case <-ctx.Done():
			return totalBytes
		case <-time.After(20 * time.Millisecond):
		}
	}

	// Tell Exotel "after the queued audio drains, fire a mark with
	// this name". We use it in Stage 5 to know when to re-enable STT.
	if markName != "" {
		mark, err := encodeMarkFrame(s.streamSid, markName)
		if err == nil {
			_ = s.writeFrame(mark)
		}
	}
	return totalBytes
}

// synthesize tries ElevenLabs first (when configured), falls back to
// Smallest on quota_exceeded. Both providers are asked for 8 kHz mono
// PCM16 directly, eliminating the resample step.
func (s *phoneSession) synthesize(ctx context.Context, text string) ([]byte, error) {
	if s.deps.ElevenLabsKey != "" && s.deps.ElevenLabsVoiceID != "" {
		pcm, err := synthElevenLabs8k(ctx, s.deps.ElevenLabsKey, s.deps.ElevenLabsVoiceID, text)
		if err == nil {
			return pcm, nil
		}
		log.Printf("wsphone: ElevenLabs failed (%v) — trying Smallest", err)
	}
	if s.deps.SmallestKey != "" {
		voice := s.deps.SmallestVoiceID
		if voice == "" {
			voice = "emily"
		}
		pcm, err := synthSmallest8k(ctx, s.deps.SmallestKey, voice, text)
		if err != nil {
			return nil, fmt.Errorf("smallest: %w", err)
		}
		return pcm, nil
	}
	return nil, fmt.Errorf("no TTS provider configured")
}

// synthElevenLabs8k requests pcm_8000 output_format directly so we can
// frame it for the carrier with no resampling. Returns the raw 16-bit
// little-endian PCM bytes.
func synthElevenLabs8k(ctx context.Context, apiKey, voiceID, text string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2_5",
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})
	url := "https://api.elevenlabs.io/v1/text-to-speech/" + voiceID + "?output_format=pcm_8000"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/pcm")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(buf))
	}
	return io.ReadAll(resp.Body)
}

// synthSmallest8k calls the same Lightning endpoint the receptionist
// HTTP TTS uses. Returns 16-bit LE PCM at 8 kHz mono — matches what
// ElevenLabs returns above, so the calling code is the same shape.
func synthSmallest8k(ctx context.Context, apiKey, voiceID, text string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{
		"text":           text,
		"voice_id":       voiceID,
		"language":       "en",
		"sample_rate":    8000,
		"add_wav_header": false,
		"speed":          1.0,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://waves-api.smallest.ai/api/v1/lightning/get_speech",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(buf))
	}
	return io.ReadAll(resp.Body)
}
