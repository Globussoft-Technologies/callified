package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func liveTestConnection(t *testing.T, send func(*websocket.Conn)) *websocket.Conn {
	t.Helper()
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		send(conn)
		<-done
	}))
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); close(done); server.Close() })
	return conn
}

func TestLiveSetupTimeoutIsDiagnosable(t *testing.T) {
	conn := liveTestConnection(t, func(*websocket.Conn) {})
	_ = conn.SetReadDeadline(time.Now().Add(25 * time.Millisecond))
	err := New(Config{}, Callbacks{}).receive(context.Background(), conn, make(chan struct{}), time.Now())
	if err == nil || !strings.Contains(err.Error(), "setup timed out") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestLiveAudioArrivesWithoutOutputTranscript(t *testing.T) {
	conn := liveTestConnection(t, func(peer *websocket.Conn) {
		_ = peer.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
		_ = peer.WriteJSON(map[string]any{"serverContent": map[string]any{"modelTurn": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"data": base64.StdEncoding.EncodeToString([]byte{1, 2})}}}}}})
	})
	got := make(chan []byte, 1)
	client := New(Config{Greeting: "Hello", Voice: "Kore"}, Callbacks{OnAudio: func(pcm []byte) { got <- pcm }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- client.receive(ctx, conn, make(chan struct{}), time.Now()) }()
	select {
	case pcm := <-got:
		if len(pcm) != 2 {
			t.Fatalf("unexpected audio: %v", pcm)
		}
	case <-time.After(time.Second):
		t.Fatal("audio waited for a transcript")
	}
	conn.Close()
	<-finished
}

func TestLiveLanguageReminderAfterInterruptionAndTurn(t *testing.T) {
	for _, event := range []string{"interrupted", "turnComplete"} {
		t.Run(event, func(t *testing.T) {
			messages := make(chan map[string]any, 1)
			conn := liveTestConnection(t, func(peer *websocket.Conn) {
				_ = peer.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
				_ = peer.WriteJSON(map[string]any{"serverContent": map[string]any{event: true}})
				_ = peer.SetReadDeadline(time.Now().Add(time.Second))
				var msg map[string]any
				_ = peer.ReadJSON(&msg)
				messages <- msg
			})
			client := New(Config{Language: "kn"}, Callbacks{CurrentReplyLanguage: func() string { return "hi" }})
			finished := make(chan error, 1)
			go func() { finished <- client.receive(context.Background(), conn, make(chan struct{}), time.Now()) }()
			select {
			case msg := <-messages:
				raw, _ := json.Marshal(msg)
				if !strings.Contains(string(raw), "CURRENT_REPLY_LANGUAGE=hi") || !strings.Contains(string(raw), `"turnComplete":false`) {
					t.Errorf("expected non-generating reminder for approved language, got %s", raw)
				}
			case <-time.After(2 * time.Second):
				t.Error("no language reminder received")
			}
			conn.Close()
			<-finished
		})
	}
}
