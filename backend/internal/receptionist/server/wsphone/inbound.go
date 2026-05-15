package wsphone

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/stt"
)

// startInbound spins up the audio-in pipeline for an active call:
//
//	caller speech (µ-law base64 in WS frames)
//	  → decode + µ-law→PCM
//	  → buffered channel
//	  → Deepgram client (linear16 @ 8 kHz, English)
//	  → final transcript callback
//	  → conversation.Manager.HandleTurn
//	  → reply text (logged in Stage 2; synthesized in Stage 3)
//
// All goroutines started here are bound to ctx; cancelling ctx (which
// the WS loop does on stop/error) drains them in order:
//
//	1. ctx cancel
//	2. close audioIn so Deepgram's send loop returns
//	3. Deepgram sends CloseStream and waits for the receive loop
//	4. inbound goroutines return
//
// Returns the audioIn channel that handleFrame writes to from the WS
// loop. Buffer size is generous (~1 s of audio at 20 ms frames) so a
// momentary STT stall doesn't drop frames; if STT stalls long enough
// to fill it, we drop instead of blocking the WS read loop (better to
// lose a fragment of audio than to hang the carrier connection).
func (s *phoneSession) startInbound(ctx context.Context) error {
	if s.deps.DeepgramKey == "" {
		log.Printf("wsphone: DEEPGRAM_API_KEY not set — STT disabled, calls will be silent on the bot side")
		return nil
	}

	// Buffered: 50 frames × ~320 bytes PCM = ~16 KB / 1 second of audio.
	// At full carrier rate (20 ms frames) STT has 1 s of slack before we
	// start dropping frames.
	s.audioIn = make(chan []byte, 50)

	dg := stt.NewClient(s.deps.DeepgramKey, "en", logger())
	dg.OnTranscript = func(text string) {
		// Filter out interim results — we only act on final transcripts.
		// Deepgram delivers both via the same callback in the existing
		// client (interim_results=true is set on connect). Empty strings
		// are interim "speech started" markers; treat as no-op.
		if text == "" {
			return
		}
		select {
		case s.transcripts <- text:
		case <-ctx.Done():
		}
	}
	dg.OnSpeechStarted = func() {
		// Barge-in: if caller starts talking while bot is mid-sentence,
		// stop the bot immediately. Without this the caller hears the
		// rest of the bot's reply on top of their own utterance and
		// the conversation feels like talking to a wall.
		if atomic.LoadInt32(&s.ttsActive) == 1 {
			log.Printf("wsphone: barge-in on %s — cancelling active TTS", s.streamSid)
			s.cancelActiveTTS()
		}
	}

	s.transcripts = make(chan string, 8)

	// Run Deepgram in its own goroutine. Cancellation: when ctx is
	// cancelled OR audioIn is closed, the Run loop returns; we then
	// close transcripts so the consumer goroutine returns too.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		dg.Run(ctx, s.audioIn)
		close(s.transcripts)
	}()

	// Transcript consumer: drives the conversation FSM. Each final
	// transcript becomes one turn. Stage 3 will replace the log line
	// with TTS synthesis of res.Message.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case text, ok := <-s.transcripts:
				if !ok {
					return
				}
				s.handleTranscript(ctx, text)
			}
		}
	}()

	log.Printf("wsphone: inbound pipeline started for stream_sid=%s", s.streamSid)
	return nil
}

// pushAudio is called from the WS read loop for each "media" frame.
// Decodes base64 µ-law, converts to PCM, and pushes into the STT
// channel. Drops the frame if the channel is full — see startInbound
// note about why dropping is preferable to blocking.
func (s *phoneSession) pushAudio(payloadB64 string) {
	if s.audioIn == nil {
		return // STT disabled
	}
	ulaw, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		// Malformed base64 in carrier-supplied data — log once per call
		// to avoid log flooding when a bug produces this for every frame.
		if !s.loggedBadPayload {
			log.Printf("wsphone: bad base64 in media payload on %s: %v", s.streamSid, err)
			s.loggedBadPayload = true
		}
		return
	}
	pcm := audio.UlawToPCM(ulaw)
	// Tee into the recording buffer regardless of whether STT keeps up
	// — the recording shouldn't be missing frames when STT stalls.
	s.appendCallerPCM(pcm)
	select {
	case s.audioIn <- pcm:
	default:
		// Drop. Counter so we can see in logs how often this happens —
		// a sustained drop rate indicates STT can't keep up.
		s.droppedFrames++
	}
}

// handleTranscript runs one turn of the conversation. The session was
// created in onStart so it always exists by the time we get here; if
// it doesn't, that's a bug worth logging once.
//
// The reply is synthesized inline (not in a goroutine) so a fast
// follow-up transcript can't interleave with the previous turn's
// audio playback — Deepgram only emits one final at a time anyway,
// but pacing one turn before starting the next keeps the audio order
// strictly causal.
//
// When the convo reaches StateEnded ("goodbye"), we mark the session
// for hangup so the WS loop closes cleanly after the final utterance.
func (s *phoneSession) handleTranscript(ctx context.Context, text string) {
	log.Printf("wsphone: transcript on %s: %q", s.streamSid, text)
	if s.deps.Manager == nil {
		log.Printf("wsphone: no manager wired; ignoring transcript")
		return
	}

	s.convoMu.Lock()
	convoID := ""
	if s.convo != nil {
		convoID = s.convo.ID
	}
	s.convoMu.Unlock()
	if convoID == "" {
		log.Printf("wsphone: transcript before convo created on %s — dropping", s.streamSid)
		return
	}

	// Capture caller utterance before the FSM runs so the recording
	// has it even if HandleTurn errors out.
	s.addTranscript("user", text)

	res := s.deps.Manager.HandleTurn(convoID, text)
	if res == nil {
		log.Printf("wsphone: HandleTurn returned nil for %s — session expired?", convoID)
		return
	}
	log.Printf("wsphone: bot reply on %s: state=%s intent=%s msg=%q",
		s.streamSid, res.State, res.Intent, res.Message)
	s.addTranscript("assistant", res.Message)

	s.speak(ctx, res.Message, fmt.Sprintf("turn-%d", time.Now().UnixMilli()))

	// "ended" means the FSM said goodbye. Flag for hangup so the WS
	// loop closes after this turn's audio finishes streaming. Also
	// nudge the WS read with a short deadline — without it, the loop
	// blocks on ReadMessage until the caller hangs up, which can take
	// 30+ seconds. Setting a 1s deadline gets a timeout error that the
	// loop treats as call-end on the next iteration.
	if string(res.State) == "ended" {
		s.requestHangup()
		_ = s.ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	}
}

// loggerSingleton returns a zap logger for use by the stt client. We
// don't need full structured logging for the receptionist demo — a
// dev-mode logger that prints to stderr is fine.
var (
	loggerOnce sync.Once
	loggerVal  *zap.Logger
)

func logger() *zap.Logger {
	loggerOnce.Do(func() {
		l, err := zap.NewDevelopment()
		if err != nil {
			loggerVal = zap.NewNop()
			return
		}
		loggerVal = l
	})
	return loggerVal
}
