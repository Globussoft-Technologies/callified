package realtime

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func hushServer(t *testing.T, handle func(*websocket.Conn)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"ready": true, "sample_rate": 8000, "frame_bytes": 160})
		handle(conn)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}
func echoHush(conn *websocket.Conn) {
	var pending []byte
	for {
		_, pcm, err := conn.ReadMessage()
		if err != nil {
			return
		}
		pending = append(pending, pcm...)
		count := len(pending) / 160 * 160
		if conn.WriteMessage(websocket.BinaryMessage, pending[:count]) != nil {
			return
		}
		pending = append(pending[:0], pending[count:]...)
	}
}
func TestHushPreservesQuietAudioAcrossOddChunks(t *testing.T) {
	input, err := newHushInput(context.Background(), hushServer(t, echoHush))
	require.NoError(t, err)
	defer input.close()
	// No speech confidence or minimum volume check; even digital silence is forwarded.
	original := make([]byte, 16000)
	for i := range original {
		if i%2 == 0 {
			original[i] = 10
		}
	}
	var output []byte
	for offset := 0; offset < len(original); {
		end := min(offset+137, len(original))
		output = append(output, input.process(original[offset:end])...)
		offset = end
	}
	require.Equal(t, original, output)
	require.Empty(t, input.pending)
}
func TestHushFailureReplaysBufferedOriginalWithoutLoss(t *testing.T) {
	endpoint := hushServer(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, nil)
		_, _, _ = conn.ReadMessage() // close with a partial frame still buffered
	})
	input, err := newHushInput(context.Background(), endpoint)
	require.NoError(t, err)
	defer input.close()
	first := bytes.Repeat([]byte{1, 0}, 40)
	second := bytes.Repeat([]byte{2, 0}, 200)
	require.Empty(t, input.process(first))
	require.Equal(t, append(append([]byte(nil), first...), second...), input.process(second))
	third := []byte{3, 0}
	require.Equal(t, third, input.process(third))
}
func TestHushMalformedResponseFailsOpen(t *testing.T) {
	input, err := newHushInput(context.Background(), hushServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1})
	}))
	require.NoError(t, err)
	defer input.close()
	original := make([]byte, 320)
	require.Equal(t, original, input.process(original))
	require.Nil(t, input.conn)
}
func TestHushAnswersPingDuringPrewarm(t *testing.T) {
	pong := make(chan struct{}, 1)
	input, err := newHushInput(context.Background(), hushServer(t, func(conn *websocket.Conn) {
		conn.SetPongHandler(func(string) error {
			select {
			case pong <- struct{}{}:
			default:
			}
			return nil
		})
		_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
		_, _, _ = conn.ReadMessage()
	}))
	require.NoError(t, err)
	defer input.close()
	select {
	case <-pong:
	case <-time.After(time.Second):
		t.Fatal("idle Hush socket did not answer keepalive")
	}
}

// Exercise the actual Live writer against separate Hush/Gemini sockets. Hush
// changes audio only; Gemini setup, turn detection and interruption mode remain.
func TestHushToLiveAudioConnection(t *testing.T) {
	endpoint := hushServer(t, echoHush)
	setup := make(chan map[string]any, 1)
	audio := make(chan []byte, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var message map[string]any
		if conn.ReadJSON(&message) != nil {
			return
		}
		setup <- message
		_ = conn.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
		for {
			var message map[string]any
			if conn.ReadJSON(&message) != nil {
				return
			}
			rt, _ := message["realtimeInput"].(map[string]any)
			a, _ := rt["audio"].(map[string]any)
			encoded, _ := a["data"].(string)
			decoded, _ := base64.StdEncoding.DecodeString(encoded)
			audio <- decoded
		}
	}))
	defer server.Close()
	original := websocket.DefaultDialer
	dialer := *original
	dialer.TLSClientConfig = &tls.Config{RootCAs: server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs}
	websocket.DefaultDialer = &dialer
	defer func() { websocket.DefaultDialer = original }()
	client := New(Config{URL: "wss" + strings.TrimPrefix(server.URL, "https"), APIKey: "test", Model: "test", Voice: "Kore", HushEnabled: true, HushURL: endpoint}, Callbacks{})
	frames := make(chan []byte, 1)
	frames <- bytes.Repeat([]byte{5, 0}, 160)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- client.Run(ctx, frames) }()
	select {
	case message := <-setup:
		detection := message["setup"].(map[string]any)["realtimeInputConfig"].(map[string]any)["automaticActivityDetection"].(map[string]any)
		require.Equal(t, false, detection["disabled"])
	case <-time.After(3 * time.Second):
		t.Fatal("missing setup")
	}
	select {
	case pcm := <-audio:
		require.Equal(t, bytes.Repeat([]byte{5, 0}, 160), pcm)
	case <-time.After(3 * time.Second):
		t.Fatal("quiet customer audio did not reach Gemini")
	}
	close(frames)
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}

func TestHushTimeoutReturnsOriginalAudio(t *testing.T) {
	input, err := newHushInput(context.Background(), hushServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage() // inference deliberately does not reply
	}))
	require.NoError(t, err)
	defer input.close()
	original := bytes.Repeat([]byte{7, 0}, 160)
	started := time.Now()
	require.Equal(t, original, input.process(original))
	require.Less(t, time.Since(started), 2*time.Second, "failed model must not stall customer audio")
	require.Nil(t, input.conn)
}
