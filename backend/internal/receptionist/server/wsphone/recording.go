package wsphone

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/receptionist/recordings"
)

// appendCallerPCM is called from the WS read loop for every inbound
// media frame, before the µ-law data is decoded for STT. We hold
// PCM (already converted in pushAudio) so we can serialize a single
// linear-PCM stream at end-of-call without re-decoding µ-law twice.
func (s *phoneSession) appendCallerPCM(pcm []byte) {
	s.recMu.Lock()
	s.callerPCM = append(s.callerPCM, pcm...)
	s.recMu.Unlock()
}

// appendBotPCM is called from speak() each time a TTS chunk has been
// queued for delivery. We accumulate the PCM (not µ-law) to keep the
// stereo channels at the same bit-depth so they can be interleaved at
// end-of-call without resampling.
func (s *phoneSession) appendBotPCM(pcm []byte) {
	s.recMu.Lock()
	s.botPCM = append(s.botPCM, pcm...)
	s.recMu.Unlock()
}

// addTranscript appends one chat bubble to the per-call transcript.
// Called once for the greeting, once per caller utterance, and once
// per bot reply. Stage 5 may add the goodbye.
func (s *phoneSession) addTranscript(role, text string) {
	s.recMu.Lock()
	s.transcript = append(s.transcript, recordings.TranscriptLine{
		Role: role,
		Text: text,
		TS:   time.Now().Unix(),
	})
	s.recMu.Unlock()
}

// flushRecording writes the captured caller + bot audio to the
// recordings store as a stereo WAV (L=caller, R=bot), with the
// transcript as a JSON sidecar. recorder_id is derived from the
// caller's phone — callers from the same number land in the same
// per-recorder directory, so the browser dashboard can list a phone
// caller's history alongside their browser-test bookings (when they
// later log into the dashboard with that recorder_id).
//
// Best-effort: errors are logged, never returned. Recording loss is
// preferable to crashing the call cleanup.
func (s *phoneSession) flushRecording() {
	s.recMu.Lock()
	caller := s.callerPCM
	bot := s.botPCM
	transcript := s.transcript
	s.recMu.Unlock()

	if len(caller) == 0 && len(bot) == 0 {
		return // nothing recorded — likely no audio sent on either side
	}
	if len(transcript) == 0 {
		// We have audio but no transcript — could happen if STT failed
		// from start to finish. Still save the audio for diagnostics.
		log.Printf("wsphone: flushRecording: no transcript but %d caller bytes and %d bot bytes",
			len(caller), len(bot))
	}

	// Build the WAV once; reused by both the receptionist-local store and
	// the dashboard's call_transcripts sink so we don't encode twice.
	wav := buildStereoWAV(caller, bot)
	durationS := float32(time.Since(s.startedAt).Seconds())

	// 1) Receptionist's own filesystem store (browser dashboard / past
	//    conversations within the receptionist UI). Nil-safe.
	if s.deps.Recordings != nil {
		recorderID := phoneRecorderID(s.from)
		id := newRecordingID()
		meta := recordings.Meta{
			ID:         id,
			RecorderID: recorderID,
			SessionID:  s.callSid,
			CreatedAt:  s.startedAt,
			DurationMS: int(time.Since(s.startedAt) / time.Millisecond),
			AudioMIME:  "audio/wav",
			Transcript: transcript,
		}
		if err := s.deps.Recordings.Save(meta, bytes.NewReader(wav)); err != nil {
			log.Printf("wsphone: flushRecording save failed for %s: %v", s.streamSid, err)
		} else {
			log.Printf("wsphone: recording saved id=%s recorder=%s caller_bytes=%d bot_bytes=%d transcript_lines=%d",
				id, recorderID, len(caller), len(bot), len(transcript))
		}
	}

	// 2) Dashboard sink — writes the WAV to RECORDINGS_DIR and inserts a
	//    call_transcripts row so the lead's "Past Conversations" modal
	//    on the campaigns dashboard picks this up the same way it does
	//    campaign calls. Fire-and-forget; the sink owns its own error
	//    handling and never blocks call teardown.
	if s.deps.PastConversations != nil && s.from != "" {
		// Receptionist transcripts use {role:"user"|"assistant", text:…}.
		// The dashboard expects {role:"AI"|"User", text:…}. Map here so
		// the sink stays schema-agnostic.
		dashTurns := make([]map[string]string, 0, len(transcript))
		for _, tl := range transcript {
			role := "User"
			if tl.Role == "assistant" || tl.Role == "AI" {
				role = "AI"
			}
			dashTurns = append(dashTurns, map[string]string{
				"role": role,
				"text": tl.Text,
			})
		}
		jsonBytes, err := json.Marshal(dashTurns)
		if err != nil {
			log.Printf("wsphone: flushRecording: json marshal failed for %s: %v", s.streamSid, err)
			return
		}
		s.deps.PastConversations.SaveReceptionistCall(
			s.from,
			s.callSid,
			wav,
			string(jsonBytes),
			"en", // receptionist language is fixed for now; pull from convo session later
			durationS,
		)
	}
}

// phoneRecorderID derives an opaque recorder_id from the caller's
// phone. We strip non-digit characters and prefix with "phone_" so the
// id is filesystem-safe (matches recordings.SafeID's character set)
// and trivially distinguishable from browser-generated random IDs.
//
// Empty / unknown phones get a random ID — the call still records,
// just without the cross-call correlation.
func phoneRecorderID(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if digits == "" {
		return newRecordingID() // 16 hex chars; satisfies SafeID
	}
	return "phone_" + digits
}

// newRecordingID returns a short random hex id (16 chars). Same shape
// as the browser path's IDs so the rest of the recordings layer
// doesn't have to special-case phone-call uploads.
func newRecordingID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// buildStereoWAV combines two mono 8 kHz PCM16 streams into a single
// stereo 8 kHz PCM16 WAV. The shorter stream is zero-padded so both
// channels are the same length — this matches the natural call shape
// where the bot stops talking before the caller hangs up.
//
// Format-tag 1 (PCM); browsers, ffmpeg, and the recordings UI all
// play this natively without any decoder plugin.
func buildStereoWAV(left, right []byte) []byte {
	// Each PCM16 sample is 2 bytes per channel. Pad whichever side is
	// shorter with zeros so the channels line up.
	leftSamples := len(left) / 2
	rightSamples := len(right) / 2
	maxSamples := leftSamples
	if rightSamples > maxSamples {
		maxSamples = rightSamples
	}

	const sampleRate = 8000
	const channels = 2
	const bitDepth = 16
	dataLen := maxSamples * channels * (bitDepth / 8)
	blockAlign := channels * bitDepth / 8
	byteRate := sampleRate * blockAlign

	var buf bytes.Buffer
	buf.Grow(44 + dataLen)
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

	// Interleave: for each sample index, emit left then right.
	// Out-of-range indices use silence (0x0000).
	for i := 0; i < maxSamples; i++ {
		var l, r int16
		if i < leftSamples {
			l = int16(binary.LittleEndian.Uint16(left[i*2 : i*2+2]))
		}
		if i < rightSamples {
			r = int16(binary.LittleEndian.Uint16(right[i*2 : i*2+2]))
		}
		_ = binary.Write(&buf, binary.LittleEndian, l)
		_ = binary.Write(&buf, binary.LittleEndian, r)
	}
	return buf.Bytes()
}

// silence the unused import warning when audio is referenced only in
// future barge-in code (Stage 5).
var _ = audio.UlawToPCM

// ensureFmtUsed silences the unused import for fmt which is referenced
// only by error-formatting helpers added later.
var _ = fmt.Sprintf
