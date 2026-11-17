package wshandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestAgentHangupClosesExotelCarrierWithoutREST(t *testing.T) {
	closed := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, err = conn.ReadMessage()
		closed <- err
	}))
	defer server.Close()
	conn := dialWS(t, server.URL)
	defer conn.Close()
	h := newTestHandler(t)
	sess := &CallSession{CallSid: "exotel-call", IsExotel: true, IsBridge: true, WS: conn}
	h.sessionsByCallSid.Store(sess.CallSid, sess)
	h.hangupBridgeCall(context.Background(), sess.CallSid, sess)
	require.True(t, sess.HangupRequested())
	select {
	case err := <-closed:
		require.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "carrier should receive a normal close: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("carrier connection remained open")
	}
}

func TestCloseExotelBridgeDoesNotCloseOtherProvidersOrRingingCalls(t *testing.T) {
	h := newTestHandler(t)
	for _, sess := range []*CallSession{
		{CallSid: "ringing", IsExotel: true, IsBridge: true},
		{CallSid: "plivo", Provider: "plivo", IsBridge: true},
		{CallSid: "ai-call", IsExotel: true},
		{CallSid: "sim", IsExotel: true, IsBridge: true, IsWebSim: true},
	} {
		h.sessionsByCallSid.Store(sess.CallSid, sess)
		require.False(t, h.CloseExotelBridge(sess.CallSid))
		require.False(t, sess.HangupRequested())
	}
	require.False(t, h.CloseExotelBridge("missing"))
}
