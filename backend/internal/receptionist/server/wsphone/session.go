package wsphone

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/globussoft/callified-backend/internal/receptionist/recordings"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

// phoneSession holds per-call state. One instance per inbound call.
// Owns the WS write side (wsMu protects concurrent writes — gorilla
// forbids concurrent writers on a single conn) and a reference to the
// conversation.Session for transcript and FSM state.
type phoneSession struct {
	deps Deps
	ws   *websocket.Conn
	wsMu sync.Mutex // serializes WS writes from any goroutine

	streamSid string    // populated from the "start" event
	callSid   string    // Exotel's call SID for logging/recording correlation
	from      string    // caller's phone, e.g. "+919876543210"
	to        string    // dialed number — receptionist's Exotel number
	startedAt time.Time

	// Inbound audio pipeline (Stage 2). audioIn carries linear-PCM
	// frames decoded from carrier µ-law; transcripts carries finalized
	// utterances back from Deepgram. Both are nil when STT is disabled.
	audioIn     chan []byte
	transcripts chan string

	// Receptionist FSM state. convo is created on the first transcript
	// (or in Stage 4, immediately after "start" for the greeting). The
	// mutex guards the lazy-init race between WS read goroutine and the
	// transcript consumer goroutine.
	convo   *session.Session
	convoMu sync.Mutex

	// wg waits for all per-call goroutines (Deepgram run, transcript
	// consumer, future TTS worker) to drain before cleanup writes the
	// recording.
	wg sync.WaitGroup

	// Diagnostics — counted across the call, logged once at cleanup.
	droppedFrames    int
	loggedBadPayload bool

	// ttsActive is 1 while a speak() call is sending frames; readers
	// check it to filter inbound transcripts that are actually the
	// bot hearing itself echo back. atomic so the WS read goroutine
	// can read without taking a lock.
	ttsActive int32

	// cancelTTS, when non-nil, cancels the in-flight speak() context.
	// On barge-in (caller speaks while bot is talking) we call this
	// to stop sending frames immediately. Guarded by ttsCancelMu so
	// concurrent speak/cancel doesn't race.
	cancelTTS   context.CancelFunc
	ttsCancelMu sync.Mutex

	// hangupRequested signals the WS loop to send a final mark and
	// close the connection on the next iteration. Set when the convo
	// reaches StateEnded.
	hangupRequested int32

	// Recording capture. callerPCM holds the linear-PCM mic stream
	// (already converted from carrier µ-law); botPCM holds bot TTS
	// PCM at the same sample rate. recMu serializes writes from the
	// WS read goroutine and the speak goroutine. transcript is the
	// chat-bubble log written to the recording sidecar.
	recMu       sync.Mutex
	callerPCM   []byte
	botPCM      []byte
	transcript  []recordings.TranscriptLine

	// Wait-for-stop synchronization. cleanup() is idempotent.
	cleanedOnce sync.Once
}

func newPhoneSession(ws *websocket.Conn, d Deps) *phoneSession {
	return &phoneSession{
		deps: d,
		ws:   ws,
	}
}

// handleFrame routes a decoded inbound frame to the right handler. This
// is hot path — called for every 20ms audio chunk — so any logging here
// would flood the logs. Per-frame logs are intentionally absent except
// in the start/stop/mark/clear paths.
func (s *phoneSession) handleFrame(ctx context.Context, f *inboundFrame) error {
	switch f.Event {
	case "connected":
		// Handshake. Nothing to do; "start" is what carries the IDs.
		log.Printf("wsphone: connected (raw=%s)", truncate(f.Raw, 200))
		return nil
	case "start":
		return s.onStart(ctx, f.Start)
	case "media":
		return s.onMedia(ctx, f.Media)
	case "mark":
		return s.onMark(ctx, f.Mark)
	case "stop":
		return s.onStop(ctx, f.Stop)
	default:
		log.Printf("wsphone: ignoring unknown event %q on %s", f.Event, s.streamSid)
		return nil
	}
}

// onStart handles the "start" event. Captures the caller's phone,
// starts the conversation FSM (so the greeting can be synthesized),
// kicks off the inbound audio pipeline, and speaks the greeting back
// to the caller. After this returns, the WS loop continues reading
// inbound media frames; replies are sent from the transcript handler.
//
// Stage 4 will replace the basic greeting with a recall-aware version
// that personalises by phone-number lookup ("Welcome back, Harsha").
func (s *phoneSession) onStart(ctx context.Context, st *startData) error {
	if st == nil {
		log.Printf("wsphone: start frame with no payload — ignoring")
		return nil
	}
	s.streamSid = st.StreamSid
	s.callSid = st.CallSid
	s.from = st.From
	s.to = st.To
	s.startedAt = time.Now()
	log.Printf("wsphone: start call_sid=%s stream_sid=%s from=%s to=%s",
		s.callSid, s.streamSid, s.from, s.to)

	// Start the conversation now so we have a session id and a real
	// greeting string. Recall lookup: if the caller's phone matches a
	// confirmed appointment, override the greeting with a personalized
	// one and pre-fill patient_name so the FSM doesn't ask "may I
	// have your name" — the bot already knows.
	if s.deps.Manager != nil {
		s.convoMu.Lock()
		convoSess, greeting := s.deps.Manager.StartCall(s.from, "en-US", "")
		s.convo = convoSess
		if s.deps.ApptSvc != nil && s.from != "" {
			if matches := s.deps.ApptSvc.FindByPhone(s.from); len(matches) > 0 {
				appt := matches[0]
				when := appt.ScheduledFor.Format("Monday at 3:04 PM")
				greeting = fmt.Sprintf(
					"Welcome back, %s. I see you have an appointment with %s on %s. How can I help you today?",
					appt.PatientName, appt.Doctor, when,
				)
				// Pre-seed the FSM's patient_name slot so the rest of
				// the conversation skips the name-prompt step.
				convoSess.Slots["patient_name"] = appt.PatientName
				convoSess.State = "awaiting_purpose"
				log.Printf("wsphone: recall hit for %s — patient=%s appt=%s",
					s.streamSid, appt.PatientName, appt.ID)
			}
		}
		s.convoMu.Unlock()
		log.Printf("wsphone: convo started for %s; greeting=%q", s.streamSid, greeting)

		if err := s.startInbound(ctx); err != nil {
			log.Printf("wsphone: startInbound failed for %s: %v", s.streamSid, err)
		}

		// Capture the greeting as the first transcript line so the
		// recording sidecar starts with the right context.
		s.addTranscript("assistant", greeting)

		// Synthesize and send the greeting in a separate goroutine so
		// the WS read loop isn't blocked on TTS network round-trip.
		// Without this, media frames arriving during synthesis would
		// pile up at the OS socket buffer.
		go s.speak(ctx, greeting, "greeting")
		return nil
	}

	// No manager wired — still start STT so logs show transcripts.
	if err := s.startInbound(ctx); err != nil {
		log.Printf("wsphone: startInbound failed for %s: %v", s.streamSid, err)
	}
	return nil
}

// onMedia handles a "media" event — one 20ms chunk of µ-law audio from
// the caller. Decoded base64 is converted to PCM and pushed into the
// STT channel; on backpressure we drop (see startInbound rationale).
func (s *phoneSession) onMedia(ctx context.Context, m *mediaData) error {
	if m == nil || m.Payload == "" {
		return nil
	}
	s.pushAudio(m.Payload)
	return nil
}

// onMark handles a "mark" event — Exotel telling us a piece of TTS
// audio has finished playing. Stage 1 stub: log only.
func (s *phoneSession) onMark(ctx context.Context, m *markData) error {
	if m == nil {
		return nil
	}
	log.Printf("wsphone: mark name=%q on %s", m.Name, s.streamSid)
	return nil
}

// onStop handles call termination. Stage 1: just log. Stages 4+ will
// drain the audio buffer, save the recording, end the conversation
// session, and update any call-log row.
func (s *phoneSession) onStop(ctx context.Context, st *stopData) error {
	reason := ""
	if st != nil {
		reason = st.Reason
	}
	log.Printf("wsphone: stop call_sid=%s stream_sid=%s reason=%q duration=%s",
		s.callSid, s.streamSid, reason, time.Since(s.startedAt))
	return nil
}

// writeFrame serializes a JSON frame over the WS, protected by wsMu so
// concurrent senders (TTS goroutine + clear/mark from main loop) don't
// interleave bytes — gorilla/websocket panics on concurrent writers.
func (s *phoneSession) writeFrame(b []byte) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return s.ws.WriteMessage(websocket.TextMessage, b)
}

// requestHangup signals the WS read loop that the conversation is done
// (e.g. caller said "goodbye" and the FSM hit StateEnded). The loop
// observes this flag in handleFrame and returns from ServeMediaStream,
// which closes the WS via defer. Idempotent — safe to call from
// multiple goroutines.
func (s *phoneSession) requestHangup() {
	atomic.StoreInt32(&s.hangupRequested, 1)
}

// hangupRequestedFlag returns whether requestHangup has been called.
// Read by the WS loop after each frame so the flag set during a
// previous turn's TTS playback takes effect on the next iteration.
func (s *phoneSession) hangupRequestedFlag() bool {
	return atomic.LoadInt32(&s.hangupRequested) == 1
}

// setTTSCancel registers the cancel-fn for the current speak() so a
// later barge-in can interrupt it. Called from speak() right after
// the per-utterance ctx is created.
func (s *phoneSession) setTTSCancel(c context.CancelFunc) {
	s.ttsCancelMu.Lock()
	s.cancelTTS = c
	s.ttsCancelMu.Unlock()
}

// clearTTSCancel removes the registered cancel-fn — called when speak()
// completes naturally so a stale cancel doesn't fire on the next turn.
func (s *phoneSession) clearTTSCancel() {
	s.ttsCancelMu.Lock()
	s.cancelTTS = nil
	s.ttsCancelMu.Unlock()
}

// cancelActiveTTS calls the registered cancel-fn (if any) to stop the
// in-flight speak() and tells Exotel to drop any queued audio. Used
// for barge-in: caller spoke while bot was talking. Idempotent.
func (s *phoneSession) cancelActiveTTS() {
	s.ttsCancelMu.Lock()
	c := s.cancelTTS
	s.cancelTTS = nil
	s.ttsCancelMu.Unlock()
	if c != nil {
		c()
	}
	// Send "clear" to Exotel so the carrier drops any queued frames
	// it hasn't yet played to the caller. Without this, the caller
	// keeps hearing the bot's tail end after they've already spoken.
	if s.streamSid != "" {
		if frame, err := encodeClearFrame(s.streamSid); err == nil {
			_ = s.writeFrame(frame)
		}
	}
}

// cleanup is called from defer when the WS loop exits. Idempotent.
// Closes audioIn so Deepgram's send loop returns; waits for all
// per-call goroutines (STT run, transcript consumer, future TTS) to
// finish before logging the final diagnostics line. Stage 4 will add
// recording flush here.
func (s *phoneSession) cleanup() {
	s.cleanedOnce.Do(func() {
		// Close the audio-in channel so Deepgram's send loop sees a
		// closed channel and exits. Safe to close multiple times? No —
		// hence cleanedOnce. nil-check because audioIn may not have
		// been set up (no DEEPGRAM_API_KEY).
		if s.audioIn != nil {
			close(s.audioIn)
		}
		// Wait for STT + consumer to drain. cleanedOnce protects us
		// from a double-close panic if cleanup is called twice.
		s.wg.Wait()

		// End the conversation session if one was created. Releases
		// the in-memory entry so it doesn't pile up across calls.
		s.convoMu.Lock()
		if s.convo != nil && s.deps.Manager != nil {
			s.deps.Manager.EndCall(s.convo.ID)
		}
		s.convoMu.Unlock()

		// Persist the audio + transcript to the recordings store. Done
		// after wg.Wait so the bot's last reply (if any) has been fully
		// synthesized and tee'd into the bot-PCM buffer before flush.
		s.flushRecording()

		dur := time.Since(s.startedAt)
		log.Printf("wsphone: cleanup stream_sid=%s call_sid=%s duration=%s dropped_frames=%d",
			s.streamSid, s.callSid, dur, s.droppedFrames)
	})
}
