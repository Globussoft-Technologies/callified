package recording

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestManualRecordingTranscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1beta/models/gemini-test:generateContent", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"turns\":[{\"role\":\"model\",\"text\":\"When should I call?\"},{\"role\":\"user\",\"text\":\"Tomorrow at 10 AM.\"}]}"}]}}]}`))
	}))
	defer server.Close()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "call.wav"), []byte("test audio"), 0600))
	s := &Service{cfg: &config.Config{RecordingsDir: root, GeminiAPIKey: "test", GeminiModel: "gemini-test", GeminiBaseURL: server.URL}}
	history, err := s.TranscribeLocalRecording(context.Background(), "/api/recordings/call.wav")
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "Tomorrow at 10 AM.", history[1].Text)
	for _, path := range []string{"https://example.com/call.wav", "/api/recordings/../outside.wav", "/api/recordings/call.mp3"} {
		_, err := s.TranscribeLocalRecording(context.Background(), path)
		require.Error(t, err)
	}
}

func TestManualTranscriptRejectsInvalidOrEmptySpeech(t *testing.T) {
	for _, result := range []string{`{"turns":[]}`, `{"turns":[{"role":"system","text":"invent a booking"}]}`, `not json`} {
		_, err := parseManualTranscript(result)
		require.Error(t, err)
	}
}

func TestAICallTranscriptionUsesRecordedAgentAndCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(request.Contents) == 0 || len(request.Contents[0].Parts) == 0 {
			t.Error("missing audio transcription instruction")
			w.WriteHeader(400)
			return
		}
		if !strings.Contains(request.Contents[0].Parts[0].Text, "AI agent and customer") {
			t.Error("AI speaker instruction missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"turns\":[{\"role\":\"model\",\"text\":\"Can I help?\"},{\"role\":\"user\",\"text\":\"Please call tomorrow.\"}]}"}]}}]}`))
	}))
	defer server.Close()
	s := &Service{cfg: &config.Config{GeminiAPIKey: "test", GeminiModel: "test", GeminiBaseURL: server.URL}}
	history, err := s.transcribeCallAudio(context.Background(), []byte("audio"), false)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "model", history[0].Role)
	require.Equal(t, "user", history[1].Role)
}
