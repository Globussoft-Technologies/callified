package wsphone

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/conversation"
	"github.com/globussoft/callified-backend/internal/receptionist/recordings"
)

// upgrader allows any origin — Exotel doesn't send Origin headers, and
// we don't trust them anyway; the actual auth boundary is the carrier
// signature on the ExoML fetch (see exotelVoice handler).
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Deps wires the per-call dependencies the handler needs. Constructed
// once at server startup and reused across calls.
type Deps struct {
	Manager    *conversation.Manager
	ApptSvc    *appointment.Service
	Recordings *recordings.Store
	// ElevenLabsKey + Voices come from env; passed in so tests can stub.
	ElevenLabsKey      string
	ElevenLabsVoiceID  string // default voice; per-call override possible
	SmallestKey        string // fallback when ElevenLabs fails
	SmallestVoiceID    string
	DeepgramKey        string
	// MaxCallSeconds aborts a runaway call. Defaults to 600 (10 min).
	MaxCallSeconds int
}

// Handler holds the WS-upgrade entrypoint and the active call registry.
// Multiple concurrent calls are allowed; each call gets its own goroutine
// pool tied to the call's ctx — when the call ends, ctx cancels and all
// per-call goroutines drain.
type Handler struct {
	deps Deps

	// activeCalls tracks per-stream-sid sessions so a takeover/monitor
	// path could find them later. Currently unused by callers but kept
	// because we'll need it for live-listen and supervisor controls.
	activeCalls sync.Map // map[streamSid]*phoneSession
}

// New constructs a Handler. Validates that we have at least one TTS
// provider configured — without one we can't speak and there's no point
// answering calls.
func New(d Deps) (*Handler, error) {
	if d.MaxCallSeconds <= 0 {
		d.MaxCallSeconds = 600
	}
	if d.ElevenLabsKey == "" && d.SmallestKey == "" {
		log.Printf("wsphone: WARNING — neither ElevenLabs nor Smallest is configured; calls will be silent")
	}
	return &Handler{deps: d}, nil
}

// ServeMediaStream handles GET /api/receptionist/media-stream — the WS
// endpoint Exotel connects to after our /exotel/voice handler returns
// ExoML. The flow:
//
//  1. Upgrade the HTTP connection to a WebSocket.
//  2. Wait for the "start" event (carries call_sid + caller's "from").
//  3. Spin up the conversation session and the audio pipelines.
//  4. Loop: read frames, decode events, drive the conversation.
//  5. On "stop" (or ctx cancel): drain audio, save recording, close.
//
// Stage 1 leaves audio in/out as TODO so the protocol layer is reviewable
// in isolation. The handler currently logs every event and acks the
// "start" with a stub greeting frame so end-to-end framing can be
// verified against the local Exotel simulator.
func (h *Handler) ServeMediaStream(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("wsphone: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(h.deps.MaxCallSeconds)*time.Second)
	defer cancel()

	sess := newPhoneSession(conn, h.deps)
	defer sess.cleanup()

	log.Printf("wsphone: WS connected from %s", r.RemoteAddr)

	// The first event Exotel sends after upgrade is "connected", followed
	// by "start". We loop until "stop" or until ctx ends. Exotel sends
	// "media" frames every 20ms — the loop must be non-blocking enough to
	// keep up. Heavy work (STT, TTS) runs in dedicated goroutines that
	// the session owns; this loop only routes events.
	for {
		select {
		case <-ctx.Done():
			log.Printf("wsphone: ctx done (%v) — closing call %s", ctx.Err(), sess.streamSid)
			return
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("wsphone: read err on call %s: %v", sess.streamSid, err)
			return
		}

		frame, err := decodeFrame(msg)
		if err != nil {
			log.Printf("wsphone: decode err: %v (msg=%s)", err, truncate(msg, 200))
			continue
		}

		if err := sess.handleFrame(ctx, frame); err != nil {
			log.Printf("wsphone: handle err on call %s: %v", sess.streamSid, err)
			return
		}

		// "stop" is the call-ended signal. After we've handled it
		// (drained audio, saved recording) we close the WS.
		if frame.Event == "stop" {
			log.Printf("wsphone: stop received for %s", sess.streamSid)
			return
		}

		// Conversation FSM hit StateEnded after a goodbye. Defer was
		// already arranged via requestHangup; closing the WS now lets
		// Exotel see EOF and end the carrier-side leg cleanly.
		if sess.hangupRequestedFlag() {
			log.Printf("wsphone: hangup requested for %s — closing WS", sess.streamSid)
			return
		}
	}
}

// FindByStreamSid is a hook for future monitor/whisper integration.
func (h *Handler) FindByStreamSid(sid string) *phoneSession {
	if v, ok := h.activeCalls.Load(sid); ok {
		return v.(*phoneSession)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
