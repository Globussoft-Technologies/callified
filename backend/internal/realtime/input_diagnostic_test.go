package realtime

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/globussoft/callified-backend/internal/audiodiag"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestInputDiagnosticsPreserveForwardedAudioWithAndWithoutHush(t *testing.T) {
	for _, mode := range []string{"raw", "hush", "hush_gain"} {
		t.Run(mode, func(t *testing.T) {
			enabled, gainEnabled := mode != "raw", mode == "hush_gain"
			pcm := make([]byte, 320)
			for i := range pcm {
				pcm[i] = byte(i)
			}
			expected := pcm
			if gainEnabled {
				expected = newInputGainController().process(pcm)
				require.NotEqual(t, pcm, expected)
			}
			received := make(chan []byte, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				var setup map[string]any
				if conn.ReadJSON(&setup) != nil {
					return
				}
				_ = conn.WriteJSON(map[string]any{"setupComplete": map[string]any{}})
				var input map[string]any
				if conn.ReadJSON(&input) != nil {
					return
				}
				encoded := input["realtimeInput"].(map[string]any)["audio"].(map[string]any)["data"].(string)
				decoded, _ := base64.StdEncoding.DecodeString(encoded)
				received <- decoded
				_ = conn.WriteJSON(map[string]any{"serverContent": map[string]any{"inputTranscription": map[string]any{"text": "What is your name?"}}})
				// Stay connected until the test cancels the client.
				_, _, _ = conn.ReadMessage()
			}))
			defer server.Close()
			original := websocket.DefaultDialer
			dialer := *original
			dialer.TLSClientConfig = &tls.Config{RootCAs: server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs}
			websocket.DefaultDialer = &dialer
			defer func() { websocket.DefaultDialer = original }()
			transcript := make(chan struct{}, 1)
			cfg := Config{URL: "wss" + strings.TrimPrefix(server.URL, "https"), APIKey: "test", Model: "test", Voice: "Kore", HushEnabled: enabled, InputGainEnabled: gainEnabled}
			if enabled {
				cfg.HushURL = hushServer(t, echoHush)
			}
			client := New(cfg, Callbacks{OnInputTranscript: func(string) { transcript <- struct{}{} }})
			capture := audiodiag.New(t.TempDir())
			require.NotNil(t, capture)
			client.SetInputDiagnostic(capture)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := make(chan []byte, 1)
			input <- pcm
			done := make(chan error, 1)
			go func() { done <- client.Run(ctx, input) }()
			select {
			case actual := <-received:
				require.Equal(t, expected, actual)
			case <-time.After(4 * time.Second):
				t.Fatal("audio did not reach peer")
			}
			select {
			case <-transcript:
			case <-time.After(4 * time.Second):
				t.Fatal("missing transcript")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				t.Fatal("client did not stop")
			}
			capture.Close()
			for _, stage := range []string{"pre_hush", "post_hush"} {
				data, err := os.ReadFile(filepath.Join(capture.Dir, stage+".pcm"))
				require.NoError(t, err)
				require.Equal(t, pcm, data)
			}
			sent, err := os.ReadFile(filepath.Join(capture.Dir, "gemini_sent.pcm"))
			require.NoError(t, err)
			require.Equal(t, expected, sent)
			if gainEnabled {
				adjusted, err := os.ReadFile(filepath.Join(capture.Dir, "post_gain.pcm"))
				require.NoError(t, err)
				require.Equal(t, expected, adjusted)
			}
			events, err := os.ReadFile(filepath.Join(capture.Dir, "events.jsonl"))
			require.NoError(t, err)
			require.Contains(t, string(events), "What is your name?")
		})
	}
}
