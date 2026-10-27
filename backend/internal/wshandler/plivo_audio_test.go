package wshandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// The socket peer captures exactly what a carrier would receive.
func plivoTestSession(t *testing.T) (*CallSession, <-chan map[string]interface{}) {
	t.Helper()
	frames := make(chan map[string]interface{}, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var frame map[string]interface{}
			if conn.ReadJSON(&frame) != nil {
				return
			}
			frames <- frame
		}
	}))
	t.Cleanup(server.Close)
	conn := dialWS(t, server.URL)
	t.Cleanup(func() { _ = conn.Close() })
	sess := NewCallSession("plivo-test", conn, zap.NewNop())
	sess.Provider, sess.IsExotel, sess.UseUlaw = "plivo", false, true
	return sess, frames
}

func readPlivoFrame(t *testing.T, frames <-chan map[string]interface{}) []byte {
	t.Helper()
	select {
	case frame := <-frames:
		require.Equal(t, "playAudio", frame["event"])
		media := frame["media"].(map[string]interface{})
		require.Equal(t, "audio/x-mulaw", media["contentType"])
		require.Equal(t, float64(8000), media["sampleRate"])
		payload, err := base64.StdEncoding.DecodeString(media["payload"].(string))
		require.NoError(t, err)
		require.Len(t, payload, 160)
		return payload
	case <-time.After(time.Second):
		t.Fatal("no Plivo audio received")
		return nil
	}
}

func TestPlivoPacketizesAcrossChunksAndFlushesTail(t *testing.T) {
	sess, frames := plivoTestSession(t)
	pcm := bytes.Repeat([]byte{0, 16}, 200) // 25 ms
	sendPlivoPCM(sess, pcm[:81], 0, false)  // even a split PCM sample is preserved
	require.Empty(t, frames)
	sendPlivoPCM(sess, pcm[81:], 0, false)
	require.Equal(t, audio.PCMToUlaw(pcm[:320]), readPlivoFrame(t, frames))
	sendPlivoPCM(sess, nil, 0, true)
	tail := make([]byte, 320)
	copy(tail, pcm[320:])
	require.Equal(t, audio.PCMToUlaw(tail), readPlivoFrame(t, frames))
	require.Empty(t, sess.plivoOutput.pcm)
}

func TestPlivoInterruptionDiscardsBufferedAudio(t *testing.T) {
	sess, frames := plivoTestSession(t)
	sendPlivoPCM(sess, bytes.Repeat([]byte{0, 16}, 80), 0, false)
	sess.playbackEpoch.Add(1)
	sendPlivoPCM(sess, nil, 0, true) // stale flush must not play the old fragment
	require.Empty(t, frames)
	pcm := make([]byte, 320)
	sendPlivoPCM(sess, pcm, 1, false)
	require.Equal(t, audio.PCMToUlaw(pcm), readPlivoFrame(t, frames))
}

func TestPlivoAudioIsPaced(t *testing.T) {
	sess, frames := plivoTestSession(t)
	start := time.Now()
	sendPlivoPCM(sess, make([]byte, 320*6), 0, false)
	require.GreaterOrEqual(t, time.Since(start), 90*time.Millisecond)
	for i := 0; i < 6; i++ {
		readPlivoFrame(t, frames)
	}
}

func TestPlivoClosingLineStillPlays(t *testing.T) {
	sess, frames := plivoTestSession(t)
	sess.RequestFinalClose()
	pcm := bytes.Repeat([]byte{0, 16}, 160)
	sendPlivoPCM(sess, pcm, sess.PlaybackEpoch(), false)
	require.Equal(t, audio.PCMToUlaw(pcm), readPlivoFrame(t, frames))
}

// Keep the browser's wire representation explicit: PCM16 in JSON; the bridge
// turns it into the same Plivo envelope used by AI playback.
func browserAudioMessage(pcm []byte) []byte {
	msg, _ := json.Marshal(map[string]string{"type": "audio", "payload": base64.StdEncoding.EncodeToString(pcm)})
	return msg
}

func TestPlivoBrowserBridgeRelaysBothDirectionsAndHangsUp(t *testing.T) {
	sess, carrierFrames := plivoTestSession(t)
	sess.IsBridge, sess.OrgID, sess.UserID = true, 7, 9
	close(sess.bridgeReady)
	sess.CallSid = "carrier-call"
	h := newTestHandler(t)
	h.cfg.JWTSecret = "test-secret"
	h.sessionsByCallSid.Store("app-call", sess)
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		h.ServeAgent(w, r)
	}))
	defer server.Close()
	ticket, err := dial.AgentTicket(h.cfg.JWTSecret, "app-call", 7, 9)
	require.NoError(t, err)
	browser := dialWS(t, server.URL+"?call_sid=app-call&ticket="+ticket)
	defer browser.Close()
	require.NoError(t, browser.SetReadDeadline(time.Now().Add(2*time.Second)))
	var msg map[string]interface{}
	require.NoError(t, browser.ReadJSON(&msg))
	require.Equal(t, "waiting", msg["status"])

	// Plivo only starts this stream after answer; its first media opens the
	// browser microphone immediately, rather than waiting for four seconds of speech.
	customer := audio.PCMToUlaw(bytes.Repeat([]byte{0, 8}, 160))
	h.handleMediaEvent(sess, map[string]interface{}{"media": map[string]interface{}{
		"payload": base64.StdEncoding.EncodeToString(customer),
	}})
	require.NoError(t, browser.ReadJSON(&msg))
	require.Equal(t, "connected", msg["status"])
	msg = nil
	require.NoError(t, browser.ReadJSON(&msg))
	require.Equal(t, "audio", msg["type"])
	require.Equal(t, base64.StdEncoding.EncodeToString(audio.UlawToPCM(customer)), msg["payload"])

	agent := bytes.Repeat([]byte{0, 16}, 160)
	require.NoError(t, browser.WriteMessage(websocket.TextMessage, browserAudioMessage(agent[:80])))
	require.NoError(t, browser.WriteMessage(websocket.TextMessage, browserAudioMessage(agent[80:])))
	require.Equal(t, audio.PCMToUlaw(agent), readPlivoFrame(t, carrierFrames))
	require.NoError(t, browser.WriteJSON(map[string]string{"type": "hangup"}))
	select {
	case <-finished:
		require.True(t, sess.HangupRequested())
	case <-time.After(time.Second):
		t.Fatal("browser hangup did not close the bridge")
	}
}

func TestAgentRejectsUnauthenticatedUpgrade(t *testing.T) {
	h := newTestHandler(t)
	h.cfg.JWTSecret = "test-secret"
	w := httptest.NewRecorder()
	h.ServeAgent(w, httptest.NewRequest("GET", "/ws/agent?call_sid=app-call", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestPlivoBridgeHydrationDoesNotStartAI(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	require.NoError(t, h.store.SetPendingCall(ctx, "carrier-call", rstore.PendingCallInfo{
		IsBridge: true, OrgID: 7, TTSLanguage: "en", TTSProvider: "gemini_live",
	}))
	sess := NewCallSession("plivo-carrier-call", nil, zap.NewNop())
	sess.Provider, sess.CallSid, sess.LivePreparationSID = "plivo", "carrier-call", "app-call"
	startedAI := false
	sess.StartSTT = func() { startedAI = true }
	sess.SendGreeting = func() { startedAI = true }
	stop := h.handleTextFrame(ctx, sess, []byte(`{"event":"start","start":{"callId":"carrier-call","streamId":"plivo-stream"}}`))
	require.False(t, stop)
	require.True(t, sess.IsBridge)
	require.False(t, startedAI)
	got, ok := h.sessionsByCallSid.Load("app-call")
	require.True(t, ok)
	require.Same(t, sess, got)
}
