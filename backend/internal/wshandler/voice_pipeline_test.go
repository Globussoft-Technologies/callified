package wshandler

import (
	"net/url"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConfigureVoicePipelineUsesSelectedProvider(t *testing.T) {
	h := &Handler{cfg: &config.Config{GeminiLiveURL: "wss://example.com/live", GeminiAPIKey: "test-key"}}
	sess := NewCallSession("web_sim_test", nil, zap.NewNop())

	sess.TTSProvider = "sarvam"
	h.configureVoicePipeline(sess)
	require.False(t, sess.GeminiLive)

	sess.TTSProvider = "gemini_live"
	h.configureVoicePipeline(sess)
	require.True(t, sess.GeminiLive)
}

func TestConfigureVoicePipelineAcceptsSeparateLiveGatewayKey(t *testing.T) {
	h := &Handler{cfg: &config.Config{GeminiLiveURL: "wss://example.com/live", GeminiLiveAPIKey: "gateway-key"}}
	sess := NewCallSession("web_sim_test", nil, zap.NewNop())
	sess.TTSProvider = "gemini_live"

	h.configureVoicePipeline(sess)
	require.True(t, sess.GeminiLive)
}

func TestValidateMediaStreamParamsAllowsGeminiLive(t *testing.T) {
	require.Empty(t, validateMediaStreamParams(url.Values{"tts_provider": {"gemini_live"}}))
}
