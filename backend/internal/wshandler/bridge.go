package wshandler

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/audio"
)

// pcmRMS returns the root-mean-square of a PCM-16 LE buffer (range 0–32767).
func pcmRMS(pcm []byte) float64 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(n))
}

// bridgeSendRealtime sends a PCM frame to the bridge channel.
// If the buffer is full it drops the oldest frame and enqueues the new one,
// keeping the relay real-time instead of accumulating stale audio.
func bridgeSendRealtime(ch chan []byte, pcm []byte) {
	select {
	case ch <- pcm:
	default:
		// Buffer full: drain one old frame, then send the latest.
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- pcm:
		default:
		}
	}
}

// ServeAgent handles /ws/agent?call_sid=XXX for browser-to-phone bridge calls.
//
// The agent's browser connects here to relay audio between their microphone and
// the customer's phone. The Exotel session must already exist (lead's phone is
// ringing or answered) and must have IsBridge=true.
//
// Protocol (JSON over WebSocket):
//
//	Server → Agent: {"type":"status","status":"waiting|connected"}
//	Server → Agent: {"type":"audio","payload":"<base64_pcm16_8k>"}  (phone → browser)
//	Server → Agent: {"type":"hangup"}                               (call ended)
//	Agent  → Server: {"type":"audio","payload":"<base64_pcm16_8k>"} (browser → phone)
func (h *Handler) ServeAgent(w http.ResponseWriter, r *http.Request) {
	callSid := strings.TrimSpace(r.URL.Query().Get("call_sid"))
	if msg := validateMonitorKey(callSid); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("agent ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	send := func(v any) {
		b, _ := json.Marshal(v)
		conn.WriteMessage(websocket.TextMessage, b) //nolint:errcheck
	}

	send(map[string]string{"type": "status", "status": "waiting"})

	// Wait up to 60 s for the Exotel session to register (call is ringing).
	sess, ok := h.lookupSession(callSid, 60*time.Second)
	if !ok {
		h.log.Warn("agent: session not found", zap.String("call_sid", callSid))
		send(map[string]string{"type": "error", "msg": "call not found — lead may not have answered yet"})
		return
	}

	// FIX (Bug 1 — race): lookupSession returns as soon as sessionsByCallSid is
	// populated, which happens BEFORE the Redis lookup that sets IsBridge=true.
	// The window is typically 1–10 ms (one Redis round-trip), but without this
	// retry we can fail spuriously on a fast browser reconnect.
	bridgeDeadline := time.Now().Add(3 * time.Second)
	for !sess.IsBridge {
		if time.Now().After(bridgeDeadline) {
			h.log.Warn("agent: session found but IsBridge never set", zap.String("call_sid", callSid))
			send(map[string]string{"type": "error", "msg": "not a browser-call session"})
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	// FIX (Bug 2 — cross-voice): reject a second connection to the same session.
	// Without this, a browser reconnect or double-click leaves the old goroutine
	// alive and both goroutines write agent audio to the same Exotel WS — the
	// customer hears two voices simultaneously.
	if !sess.agentConnected.CompareAndSwap(false, true) {
		h.log.Warn("agent: duplicate connection rejected", zap.String("call_sid", callSid))
		send(map[string]string{"type": "error", "msg": "another agent tab is already connected to this call"})
		return
	}
	defer sess.agentConnected.Store(false)

	// FIX (Bug 3 — data race): capture StreamSid and UseUlaw once, right here,
	// after IsBridge is confirmed true. Both fields were set earlier in the same
	// handleStartEvent call that set IsBridge, so they are final by this point.
	// Reading them inside the goroutine on every frame iteration without a
	// happens-before guarantee would be flagged by the race detector.
	streamSid := sess.StreamSid
	useUlaw := sess.UseUlaw
	frameKey := "stream_sid"
	if useUlaw {
		frameKey = "streamSid"
	}

	h.log.Info("agent browser connected — waiting for customer to answer",
		zap.String("call_sid", callSid),
		zap.String("stream_sid", streamSid),
		zap.Bool("use_ulaw", useUlaw),
	)

	agentDone := make(chan struct{})

	// Agent → Exotel: read base64 PCM-16 from agent browser and send to Exotel.
	// Goroutine starts immediately so the WebSocket stays alive and agent
	// disconnection is detected even while we wait for the customer to answer.
	// Audio processing only begins after "connected" is sent below.
	var customerAudioReady atomic.Bool
	go func() {
		defer close(agentDone)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Discard frames that arrive before "connected" is sent.
			// In practice the browser's connectedRef guard prevents sending
			// any audio before it receives "connected", so this is a safety net.
			if !customerAudioReady.Load() {
				continue
			}
			var data map[string]interface{}
			if json.Unmarshal(msg, &data) != nil {
				continue
			}
			if data["type"] != "audio" {
				continue
			}
			payload, _ := data["payload"].(string)
			if payload == "" {
				continue
			}
			rawPCM, decErr := base64.StdEncoding.DecodeString(payload)
			if decErr != nil || len(rawPCM) == 0 {
				continue
			}
			// Convert PCM-16 → μ-law when the Exotel session uses μ-law encoding.
			// (Voicebot applet uses PCM-16 directly; Passthru/Twilio uses μ-law.)
			var audioBytes []byte
			if useUlaw {
				audioBytes = audio.PCMToUlaw(rawPCM)
			} else {
				audioBytes = rawPCM
			}
			frame, _ := json.Marshal(map[string]interface{}{
				"event":   "media",
				frameKey:  streamSid,
				"media":   map[string]string{"payload": base64.StdEncoding.EncodeToString(audioBytes)},
			})
			sess.SendText(frame) //nolint:errcheck
		}
	}()

	// ── Wait for customer to answer, then relay audio ───────────────────────
	//
	// "connected" is sent to the browser only after we're confident the customer
	// has actually answered. The browser's connectedRef guard means it won't send
	// any mic audio until it receives "connected" — so delaying "connected" prevents
	// browser audio from accumulating in Exotel's buffer during ringing.
	//
	// Three signals can open the gate (first one wins):
	//  1. Customer speech detected in BridgeCh (RMS > 500) — primary, fast (~1s)
	//  2. Exotel "in-progress" webhook via Redis — secondary
	//  3. 30s absolute fallback (customer silent throughout or webhook not configured)

	// Redis webhook signal (fires if Exotel status webhook arrives)
	answeredCh := make(chan struct{}, 1)
	go func() {
		if h.store.WaitBridgeAnswered(r.Context(), callSid, 30*time.Second) {
			select {
			case answeredCh <- struct{}{}:
			default:
			}
		}
	}()

	fallback := time.NewTimer(30 * time.Second)
	defer fallback.Stop()
	connectedSent := false

	// Exotel → Agent relay loop. Also detects customer speech to trigger "connected".
	for {
		select {
		case pcm, chOk := <-sess.BridgeCh:
			if !chOk {
				send(map[string]string{"type": "hangup"})
				return
			}
			// Always forward customer audio so agent can hear ringing/speech.
			outMsg, _ := json.Marshal(map[string]string{
				"type":    "audio",
				"payload": base64.StdEncoding.EncodeToString(pcm),
			})
			if err := conn.WriteMessage(websocket.TextMessage, outMsg); err != nil {
				return
			}
			// Customer speech detected → open agent mic gate immediately.
			// During ringing Exotel sends near-silence (RMS < 50); real speech
			// from an answered customer is reliably above 500.
			if !connectedSent && pcmRMS(pcm) > 500 {
				connectedSent = true
				customerAudioReady.Store(true)
				fallback.Stop()
				h.log.Info("bridge: customer speech detected — sending connected",
					zap.String("call_sid", callSid))
				send(map[string]string{"type": "status", "status": "connected"})
			}

		case <-answeredCh:
			if !connectedSent {
				connectedSent = true
				customerAudioReady.Store(true)
				fallback.Stop()
				h.log.Info("bridge: webhook signal — sending connected",
					zap.String("call_sid", callSid))
				send(map[string]string{"type": "status", "status": "connected"})
			}

		case <-fallback.C:
			if !connectedSent {
				connectedSent = true
				customerAudioReady.Store(true)
				h.log.Info("bridge: 30s fallback — sending connected",
					zap.String("call_sid", callSid))
				send(map[string]string{"type": "status", "status": "connected"})
			}

		case <-agentDone:
			return
		}
	}
}
