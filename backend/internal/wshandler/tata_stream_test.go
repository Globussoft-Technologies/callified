package wshandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/alicebob/miniredis/v2"
	"github.com/globussoft/callified-backend/internal/audio"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTataStartBoundToCarrierIdentity(t *testing.T) {
	info := &rstore.TataStreamContext{CallID: "call-a", From: "+919999999999", To: "+918888888888"}
	good := map[string]interface{}{"start": map[string]interface{}{"callSid": "call-a", "from": info.From, "to": info.To, "org_id": float64(999)}}
	require.NoError(t, validateTataStart(info, good))
	good["start"].(map[string]interface{})["callSid"] = "call-b"
	require.Error(t, validateTataStart(info, good))
	good["start"].(map[string]interface{})["callSid"] = "call-a"
	good["start"].(map[string]interface{})["to"] = "+917777777777"
	require.Error(t, validateTataStart(info, good))
	require.Error(t, validateTataStart(info, map[string]interface{}{}))
}

func TestVerifiedTataContextOverridesUntrustedSessionIdentity(t *testing.T) {
	sess := &CallSession{OrgID: 999, LeadID: 888, CampaignID: 777, IsInbound: false}
	info := &rstore.TataStreamContext{CallID: "carrier-a", AccountID: 2, Inbound: true, Pending: rstore.PendingCallInfo{OrgID: 7, LeadID: 8, CampaignID: 9, Phone: "+919999999999", TTSLanguage: "hi"}}
	applyVerifiedTataContext(sess, info)
	require.EqualValues(t, 7, sess.OrgID)
	require.EqualValues(t, 8, sess.LeadID)
	require.EqualValues(t, 9, sess.CampaignID)
	require.Equal(t, "carrier-a", sess.CallSid)
	require.True(t, sess.IsInbound)
	// A direct call to the legacy start handler cannot hydrate an unauthenticated Tata session.
	unverified := &CallSession{Provider: "tata"}
	(&Handler{}).handleStartEvent(context.Background(), unverified, map[string]interface{}{"start": map[string]interface{}{"org_id": float64(999), "to": "+919999999999"}})
	require.Zero(t, unverified.OrgID)
}
func TestTataWebsocketRejectsMissingAndReplayedTickets(t *testing.T) {
	server := miniredis.RunT(t)
	s := rstore.New("redis://"+server.Addr(), zap.NewNop())
	h := &Handler{store: s, log: zap.NewNop()}
	for _, path := range []string{"/media-stream/tata?org_id=999&direction=inbound", "/media-stream?provider=tata&org_id=999"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 401, w.Code)
	}
	token, err := s.IssueTataStreamTicket(context.Background(), rstore.TataStreamContext{CallID: "a", Inbound: true, AccountID: 2, Pending: rstore.PendingCallInfo{OrgID: 1}})
	require.NoError(t, err)
	_, err = s.ConsumeTataStreamTicket(context.Background(), token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/media-stream/tata?ticket="+token, nil))
	require.Equal(t, 401, w.Code)
}

func TestVerifiedTataContextPreservesBrowserMode(t *testing.T) {
	for _, bridge := range []bool{true, false} {
		sess := NewCallSession("tata_test", nil, zap.NewNop())
		applyVerifiedTataContext(sess, &rstore.TataStreamContext{CallID: "carrier", Pending: rstore.PendingCallInfo{IsBridge: bridge, ExotelCallSid: "ref", OrgID: 7, UserID: 9}})
		require.Equal(t, bridge, sess.IsBridge)
		require.Equal(t, "ref", sess.CallSid)
		require.Equal(t, int64(7), sess.OrgID)
		if bridge {
			sess.TTSProvider = "gemini_live"
			sess.GeminiLive = true
			(&Handler{}).configureVoicePipeline(sess) // No AI configuration or key required.
			require.False(t, sess.GeminiLive)
			require.Nil(t, sess.TTSInstance())
		}
	}
}

func TestVerifiedTataBrowserStartSkipsAIAndRelaysCustomerAudio(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			close(done)
			return
		}
		defer conn.Close()
		defer close(done)
		sess := NewCallSession("tata_test", conn, zap.NewNop())
		sess.Provider = "tata"
		sess.VerifiedTata = &rstore.TataStreamContext{CallID: "carrier", Pending: rstore.PendingCallInfo{IsBridge: true, ExotelCallSid: "ref", OrgID: 7, UserID: 9}}
		applyVerifiedTataContext(sess, sess.VerifiedTata)
		started, greeted := false, false
		sess.StartSTT = func() { started = true }
		sess.SendGreeting = func() { greeted = true }
		select {
		case <-sess.bridgeReady:
			t.Error("bridge ready before validation")
		default:
		}
		h := &Handler{log: zap.NewNop()}
		event := map[string]interface{}{"start": map[string]interface{}{"callId": "carrier", "streamSid": "stream", "mediaFormat": map[string]interface{}{"encoding": "mulaw"}}}
		h.handleVerifiedTataStart(context.Background(), sess, event)
		require.True(t, sess.TataStartVerified)
		require.True(t, sess.IsBridge)
		require.False(t, started)
		require.False(t, greeted)
		require.Nil(t, sess.StartSTT)
		require.Nil(t, sess.SendGreeting)
		select {
		case <-sess.bridgeReady:
		default:
			t.Error("bridge not ready after validation")
		}
		h.handleVerifiedTataStart(context.Background(), sess, event) // repeated start is harmless
		stored, ok := h.sessionsByCallSid.Load("ref")
		require.True(t, ok)
		require.Same(t, sess, stored)
		encoded := bytes.Repeat([]byte{0x80}, 160)
		h.handleMediaEvent(sess, map[string]interface{}{"media": map[string]interface{}{"payload": base64.StdEncoding.EncodeToString(encoded)}})
		select {
		case got := <-sess.BridgeCh:
			require.Equal(t, audio.UlawToPCM(encoded), got)
		default:
			t.Error("customer audio not relayed")
		}
		require.Empty(t, sess.AudioIn)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	<-done
}
