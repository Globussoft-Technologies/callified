// Package wsphone implements the Exotel WebSocket inbound pipeline for
// the AI receptionist. When a caller dials the receptionist's Exotel
// number, Exotel fetches our ExoML, opens a WebSocket to /media-stream,
// and streams 8 kHz µ-law audio in both directions. This package owns
// the protocol framing, audio codec conversion, STT/TTS pipeline, and
// per-call recording — entirely separate from the campaign wshandler
// package so changes here cannot regress outbound dial flows.
//
// Protocol reference: https://developer.exotel.com/api/voice-streaming
//
// Frame format (each WS message is one JSON object):
//
//	{"event":"connected", "protocol":"...", "version":"..."}             — handshake
//	{"event":"start",     "start":{"call_sid":"…","stream_sid":"…",
//	                               "from":"+91…","to":"+91…",
//	                               "media_format":{"encoding":"audio/x-mulaw",
//	                                               "sample_rate":8000,
//	                                               "channels":1}}}        — call started
//	{"event":"media",     "stream_sid":"…",
//	                      "media":{"chunk":"…","timestamp":"…",
//	                               "payload":"<base64 ulaw>"}}            — audio in/out
//	{"event":"mark",      "stream_sid":"…", "mark":{"name":"…"}}          — playback mark
//	{"event":"clear",     "stream_sid":"…"}                               — drop queued audio
//	{"event":"stop",      "stream_sid":"…", "stop":{"reason":"…"}}        — call ended
//
// All payloads are base64-encoded 8 kHz mono µ-law. We decode on input
// (µ-law → linear PCM16 for Deepgram) and re-encode on output (PCM16 from
// ElevenLabs/Smallest → µ-law for the carrier).
package wsphone

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// inboundFrame is what we receive from Exotel. We only decode the fields
// we actually use; unknown fields are ignored. SequenceNumber is in the
// "start" frame for reconnection logic but we don't currently retry.
type inboundFrame struct {
	Event     string          `json:"event"`
	StreamSid string          `json:"stream_sid,omitempty"`
	Start     *startData      `json:"start,omitempty"`
	Media     *mediaData      `json:"media,omitempty"`
	Mark      *markData       `json:"mark,omitempty"`
	Stop      *stopData       `json:"stop,omitempty"`
	Raw       json.RawMessage `json:"-"` // populated for debugging unknown events
}

type startData struct {
	CallSid     string      `json:"call_sid"`
	StreamSid   string      `json:"stream_sid"`
	AccountSid  string      `json:"account_sid"`
	From        string      `json:"from"`
	To          string      `json:"to"`
	MediaFormat mediaFormat `json:"media_format"`
}

type mediaFormat struct {
	Encoding   string `json:"encoding"`    // "audio/x-mulaw"
	SampleRate int    `json:"sample_rate"` // 8000
	Channels   int    `json:"channels"`    // 1
}

type mediaData struct {
	Chunk     string `json:"chunk"`     // monotonically increasing chunk id
	Timestamp string `json:"timestamp"` // milliseconds since start (string per Exotel)
	Payload   string `json:"payload"`   // base64 µ-law bytes
}

type markData struct {
	Name string `json:"name"`
}

type stopData struct {
	Reason string `json:"reason"`
}

// outboundMedia is what we send to Exotel — base64 µ-law payload wrapped
// in the same envelope. StreamSid must match the one Exotel sent us in
// the "start" event, otherwise the carrier silently drops the frame.
type outboundMedia struct {
	Event     string                 `json:"event"`
	StreamSid string                 `json:"stream_sid"`
	Media     map[string]interface{} `json:"media,omitempty"`
	Mark      map[string]interface{} `json:"mark,omitempty"`
}

// decodeFrame parses one WS text message into an inboundFrame. Returns
// the frame plus an error if the JSON is malformed. Callers should treat
// unknown event types as a no-op (don't error) — Exotel adds new event
// types over time and we want forward compatibility.
func decodeFrame(msg []byte) (*inboundFrame, error) {
	var f inboundFrame
	if err := json.Unmarshal(msg, &f); err != nil {
		return nil, fmt.Errorf("decodeFrame: %w", err)
	}
	f.Raw = msg
	return &f, nil
}

// encodeMediaFrame builds an outbound "media" frame from raw µ-law bytes.
// The carrier expects ulaw bytes already at 8 kHz mono — caller is
// responsible for sample-rate conversion before calling this.
func encodeMediaFrame(streamSid string, ulaw []byte) ([]byte, error) {
	frame := outboundMedia{
		Event:     "media",
		StreamSid: streamSid,
		Media: map[string]interface{}{
			"payload": base64.StdEncoding.EncodeToString(ulaw),
		},
	}
	return json.Marshal(frame)
}

// encodeMarkFrame asks Exotel to emit a "mark" event after the queued
// audio up to this point has finished playing. We use this to know
// when the bot has finished speaking so we can re-enable STT without
// the bot hearing itself.
func encodeMarkFrame(streamSid, name string) ([]byte, error) {
	frame := outboundMedia{
		Event:     "mark",
		StreamSid: streamSid,
		Mark:      map[string]interface{}{"name": name},
	}
	return json.Marshal(frame)
}

// encodeClearFrame tells Exotel to drop any queued audio it hasn't
// played yet. Used for barge-in: when the caller starts speaking while
// the bot is mid-sentence, we cancel the rest of that sentence so the
// bot stops talking.
func encodeClearFrame(streamSid string) ([]byte, error) {
	frame := outboundMedia{Event: "clear", StreamSid: streamSid}
	return json.Marshal(frame)
}
