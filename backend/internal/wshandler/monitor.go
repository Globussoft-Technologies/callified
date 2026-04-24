package wshandler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// lookupSession resolves a monitor-WS key to an active CallSession. The key
// may be either a stream_sid or a call_sid. The call_sid index is populated
// only after the carrier sends the "start" event for an outbound dial, so we
// poll for up to maxWait to cover the ringing gap between /api/manual-call
// returning and the media stream opening.
func (h *Handler) lookupSession(key string, maxWait time.Duration) (*CallSession, bool) {
	deadline := time.Now().Add(maxWait)
	for {
		if raw, ok := h.sessions.Load(key); ok {
			return raw.(*CallSession), true
		}
		if raw, ok := h.sessionsByCallSid.Load(key); ok {
			return raw.(*CallSession), true
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ServeMonitor handles /ws/monitor/{key} where key is a stream_sid OR call_sid.
// External consumers connect here to:
//   - Receive live transcripts: {"type":"transcript","role":"user|agent","text":"..."}
//   - Receive live audio chunks: {"type":"audio","role":"user|agent","format":"...","payload":"<base64>"}
//   - Inject whispers:           {"action":"whisper","text":"hint for AI"}
//   - Trigger takeover:          {"action":"takeover"}
//   - Send audio during takeover:{"action":"audio_chunk","payload":"<base64>"}
//
// Accepting call_sid lets /api/manual-call return a URL the client can open
// immediately, before the carrier has dialled through to /media-stream.
func (h *Handler) ServeMonitor(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/ws/monitor/")
	if key == "" {
		http.Error(w, "stream_sid or call_sid required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("monitor ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Try an immediate lookup first; if the caller is monitoring by call_sid
	// on an outbound dial we may need to wait for the media stream to open.
	sess, ok := h.lookupSession(key, 30*time.Second)
	if !ok {
		h.log.Warn("monitor: session not found", zap.String("key", key))
		conn.WriteMessage( //nolint:errcheck
			websocket.TextMessage,
			[]byte(`{"error":"session not found"}`),
		)
		return
	}

	// Use the session's actual stream_sid for Redis-backed ops (whispers,
	// takeover) since those keys always live under stream_sid.
	streamSid := sess.StreamSid

	sess.AddMonitor(conn)
	defer sess.RemoveMonitor(conn)

	h.log.Info("monitor connected",
		zap.String("key", key),
		zap.String("stream_sid", streamSid),
	)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err != nil {
			continue
		}

		switch data["action"] {
		case "whisper":
			text, _ := data["text"].(string)
			if text != "" {
				h.store.PushWhisper(r.Context(), streamSid, text)
				h.log.Info("monitor whisper injected",
					zap.String("stream_sid", streamSid),
					zap.String("text", text),
				)
			}

		case "takeover":
			// Set Redis takeover flag and cancel any active TTS immediately.
			// After this, processTranscript will skip the LLM for this session.
			h.store.SetTakeover(r.Context(), streamSid, true)
			sess.CancelActiveTTS()
			if sess.IsExotel {
				sendClearEvent(sess)
			}
			h.log.Info("monitor takeover activated", zap.String("stream_sid", streamSid))

		case "audio_chunk":
			// Manager sends base64 audio directly to the phone (takeover mode).
			// Only forwarded if takeover is active.
			if !h.store.GetTakeover(r.Context(), streamSid) {
				continue
			}
			payload, _ := data["payload"].(string)
			if payload == "" {
				continue
			}
			// Validate it's valid base64 before forwarding
			if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
				continue
			}
			frame, _ := json.Marshal(map[string]interface{}{
				"event":     "media",
				"streamSid": streamSid,
				"media":     map[string]string{"payload": payload},
			})
			sess.SendText(frame) //nolint:errcheck
		}
	}

	h.log.Info("monitor disconnected", zap.String("stream_sid", streamSid))
}
