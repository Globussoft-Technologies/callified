package realtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLivePCMFormatValidation(t *testing.T) {
	for _, value := range []string{"audio/mpeg", "audio/pcm;rate=bad", "audio/pcm;rate=0", "audio/pcm;channels=2"} {
		if _, err := livePCMRate(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	rate, err := livePCMRate("audio/pcm;rate=16000")
	if err != nil || rate != 16000 {
		t.Fatalf("%d %v", rate, err)
	}
}

func TestLiveReceiveNormalizesDeclaredAudioRate(t *testing.T) {
	input := make([]byte, 16000*2) // One second of 16 kHz mono PCM16.
	conn := liveTestConnection(t, func(peer *websocket.Conn) {
		_ = peer.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
		_ = peer.WriteJSON(map[string]any{"serverContent": map[string]any{"modelTurn": map[string]any{
			"parts": []any{map[string]any{"inlineData": map[string]any{
				"mimeType": "audio/pcm;rate=16000", "data": base64.StdEncoding.EncodeToString(input),
			}}},
		}}})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var output []byte
	client := New(Config{}, Callbacks{OnAudio: func(pcm []byte) { output = pcm; cancel() }})
	err := client.receive(ctx, conn, make(chan struct{}), time.Now())
	if err != context.Canceled {
		t.Fatalf("unexpected receive result: %v", err)
	}
	if count := len(output) / 2; count < 23997 || count > 24000 {
		t.Fatalf("expected one second at 24 kHz, got %d samples", count)
	}
}

func TestLiveReceiveRejectsUnsupportedAudioBeforePlayback(t *testing.T) {
	for _, format := range []string{"audio/mpeg", "audio/pcm;rate=bad", "audio/pcm;channels=2"} {
		t.Run(format, func(t *testing.T) {
			conn := liveTestConnection(t, func(peer *websocket.Conn) {
				_ = peer.WriteJSON(map[string]any{"serverContent": map[string]any{"modelTurn": map[string]any{
					"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": format, "data": "AQI="}}},
				}}})
			})
			called := false
			client := New(Config{}, Callbacks{OnAudio: func([]byte) { called = true }})
			err := client.receive(context.Background(), conn, make(chan struct{}), time.Now())
			if err == nil || !strings.Contains(err.Error(), "gemini live:") || called {
				t.Fatalf("invalid format reached playback: format=%s error=%v called=%v", format, err, called)
			}
		})
	}
}

func TestLivePCMNormalizationAcrossChunks(t *testing.T) {
	for _, rate := range []int{8000, 16000, 24000, 44100, 48000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			input := make([]byte, rate*2)
			for i := range input {
				input[i] = byte(i)
			}
			var whole, split livePCMNormalizer
			want, _ := whole.process(input, rate)
			var got []byte
			for start := 0; start < len(input); start += 206 {
				end := start + 206
				if end > len(input) {
					end = len(input)
				}
				part, err := split.process(input[start:end], rate)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, part...)
			}
			if !bytes.Equal(want, got) {
				t.Fatalf("chunk mismatch at %d", rate)
			}
			if count := len(got) / 2; count < 23997 || count > 24000 {
				t.Fatalf("duration changed at %d: %d", rate, count)
			}
		})
	}
}
