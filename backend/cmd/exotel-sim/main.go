// Package main implements a fake Exotel WebSocket client for testing
// the receptionist's wsphone pipeline without burning real Exotel call
// minutes. Connects to a local audiod instance, sends the same frame
// sequence Exotel does (connected → start → media×N → stop), and prints
// any reply frames the server sends back.
//
// Usage:
//
//	# Terminal 1: start audiod with TTS keys configured
//	cd backend && go run ./cmd/audiod
//
//	# Terminal 2: simulate a call
//	go run ./cmd/exotel-sim -addr ws://127.0.0.1:8011/api/receptionist/media-stream
//
// Audio source for the "media" frames defaults to silence (160 zero
// bytes per frame × 50 frames = 1 second of µ-law silence). Pass
// -wav <path> to replay a real µ-law-8k wav file instead.
package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	addr := flag.String("addr", "ws://127.0.0.1:8011/api/receptionist/media-stream", "WS URL")
	from := flag.String("from", "+919876543210", "caller phone")
	to := flag.String("to", "+919513886363", "called phone")
	frames := flag.Int("frames", 100, "number of media frames to send (20 ms each)")
	wavPath := flag.String("wav", "", "path to 8 kHz µ-law mono raw audio (optional; default is silence)")
	saveOut := flag.String("save-out", "", "if set, write received bot audio to this WAV file (8 kHz µ-law)")
	holdMs := flag.Int("hold-ms", 0, "hold the connection open for this many ms after sending stop (lets bot finish replying)")
	flag.Parse()

	u, err := url.Parse(*addr)
	if err != nil {
		log.Fatalf("bad -addr: %v", err)
	}

	log.Printf("connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Optional receive-side audio capture. Decoded µ-law bytes from
	// each "media" frame the server sends back are concatenated and
	// dumped as a WAV at exit, so we can hear the bot's TTS without
	// running the full Exotel stack.
	var (
		outMu      sync.Mutex
		outUlawBuf []byte
	)

	// Receive goroutine — print any frames the server sends back.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				log.Printf("recv closed: %v", err)
				return
			}
			log.Printf("← %s", truncate(string(msg), 200))

			if *saveOut != "" {
				// Look at the frame for a media payload; if found,
				// decode and append to the bot-audio buffer.
				var f struct {
					Event string `json:"event"`
					Media struct {
						Payload string `json:"payload"`
					} `json:"media"`
				}
				if err := json.Unmarshal(msg, &f); err == nil && f.Event == "media" && f.Media.Payload != "" {
					if pcm, err := base64.StdEncoding.DecodeString(f.Media.Payload); err == nil {
						outMu.Lock()
						outUlawBuf = append(outUlawBuf, pcm...)
						outMu.Unlock()
					}
				}
			}
		}
	}()

	// Match Exotel's frame ordering: connected, start, media×N, stop.
	send(c, map[string]any{
		"event":    "connected",
		"protocol": "Call",
		"version":  "1.0.0",
	})
	streamSid := fmt.Sprintf("sim_%d", time.Now().UnixMilli())
	send(c, map[string]any{
		"event":      "start",
		"stream_sid": streamSid,
		"start": map[string]any{
			"call_sid":    "sim_call_" + streamSid,
			"stream_sid":  streamSid,
			"account_sid": "globussoft3",
			"from":        *from,
			"to":          *to,
			"media_format": map[string]any{
				"encoding":    "audio/x-mulaw",
				"sample_rate": 8000,
				"channels":    1,
			},
		},
	})

	// Build per-frame payload. 8kHz µ-law mono × 20ms = 160 bytes/frame.
	const frameBytes = 160
	var audio []byte
	if *wavPath != "" {
		b, err := os.ReadFile(*wavPath)
		if err != nil {
			log.Fatalf("read wav: %v", err)
		}
		audio = b
	} else {
		// 0xff is µ-law silence (linear 0).
		audio = make([]byte, frameBytes**frames)
		for i := range audio {
			audio[i] = 0xff
		}
	}

	for i := 0; i < *frames; i++ {
		off := i * frameBytes
		if off+frameBytes > len(audio) {
			break
		}
		chunk := audio[off : off+frameBytes]
		send(c, map[string]any{
			"event":      "media",
			"stream_sid": streamSid,
			"media": map[string]any{
				"chunk":     fmt.Sprintf("%d", i+1),
				"timestamp": fmt.Sprintf("%d", i*20),
				"payload":   base64.StdEncoding.EncodeToString(chunk),
			},
		})
		time.Sleep(20 * time.Millisecond)
	}

	// Hold the connection open before sending stop, if requested.
	// Useful when we want to capture all of the bot's TTS reply (which
	// is paced 20 ms/frame) before tearing down the call.
	if *holdMs > 0 {
		select {
		case <-time.After(time.Duration(*holdMs) * time.Millisecond):
		case <-done:
		}
	}

	send(c, map[string]any{
		"event":      "stop",
		"stream_sid": streamSid,
		"stop":       map[string]any{"reason": "client_hangup"},
	})

	// Give the server a moment to flush, then close. Ctrl-C also works.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	select {
	case <-stop:
	case <-time.After(2 * time.Second):
	case <-done:
	}

	// Write the captured bot audio to disk as a minimal WAV (µ-law @
	// 8 kHz mono) so we can `afplay` / VLC / ffplay it locally.
	if *saveOut != "" {
		outMu.Lock()
		buf := outUlawBuf
		outMu.Unlock()
		if len(buf) > 0 {
			if err := writeMulawWAV(*saveOut, buf); err != nil {
				log.Printf("save-out: %v", err)
			} else {
				log.Printf("save-out: wrote %d bytes of µ-law audio to %s", len(buf), *saveOut)
			}
		} else {
			log.Printf("save-out: no audio received (server didn't synthesize anything)")
		}
	}
	log.Printf("done")
}

// writeMulawWAV emits a RIFF/WAVE file with format-tag 7 (CCITT µ-law),
// 8 kHz mono, suitable for direct playback in any player that supports
// µ-law WAV. Avoids needing a separate µ-law→PCM decode step locally.
func writeMulawWAV(path string, ulaw []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataLen := uint32(len(ulaw))
	// RIFF header
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36+dataLen)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVE")); err != nil {
		return err
	}
	// fmt chunk
	if _, err := f.Write([]byte("fmt ")); err != nil {
		return err
	}
	_ = binary.Write(f, binary.LittleEndian, uint32(16))     // fmt chunk size
	_ = binary.Write(f, binary.LittleEndian, uint16(7))      // 7 = µ-law
	_ = binary.Write(f, binary.LittleEndian, uint16(1))      // mono
	_ = binary.Write(f, binary.LittleEndian, uint32(8000))   // sample rate
	_ = binary.Write(f, binary.LittleEndian, uint32(8000))   // byte rate
	_ = binary.Write(f, binary.LittleEndian, uint16(1))      // block align
	_ = binary.Write(f, binary.LittleEndian, uint16(8))      // bits per sample
	// data chunk
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	_ = binary.Write(f, binary.LittleEndian, dataLen)
	_, err = f.Write(ulaw)
	return err
}

func send(c *websocket.Conn, v map[string]any) {
	b, _ := json.Marshal(v)
	if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
		log.Fatalf("send: %v", err)
	}
	log.Printf("→ %s", truncate(string(b), 200))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return strings.ReplaceAll(s[:n], "\n", " ") + "…"
	}
	return strings.ReplaceAll(s, "\n", " ")
}
