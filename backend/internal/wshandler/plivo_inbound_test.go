package wshandler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPlivoIncomingHydratesBeforeStartingReceptionist(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	pending := rstore.PendingCallInfo{IsInbound: true, OrgID: 7, ExotelAccountID: 3,
		PlivoContextID: "plivo_in_3_call", ExotelCallSid: "carrier-call", Phone: "+919123456789", TTSLanguage: "en"}
	require.NoError(t, h.store.SetPendingCall(ctx, "carrier-call", pending))
	sess := NewCallSession("plivo-carrier-call", nil, zap.NewNop())
	sess.Provider, sess.CallSid, sess.LivePreparationSID = "plivo", "carrier-call", pending.PlivoContextID
	sess.OrgID = 7
	started := false
	sess.StartSTT = func() {
		started = true
		require.True(t, sess.IsInbound, "incoming direction must be set before AI starts")
		require.EqualValues(t, 7, sess.OrgID)
	}
	stop := h.handleTextFrame(ctx, sess, []byte(`{"event":"start","start":{"callId":"carrier-call","streamId":"incoming-stream"}}`))
	require.False(t, stop)
	require.True(t, started)
	require.False(t, sess.IsBridge)
	require.Equal(t, pending.Phone, sess.LeadPhone)
	require.EqualValues(t, 0, sess.CampaignID)
	h.applyInboundReceptionistPrompt(sess)
	require.Contains(t, sess.SystemPrompt, "This is an incoming voice call")
	require.NotEmpty(t, sess.GreetingText)
}

func TestPlivoIncomingNeverFallsBackToAnotherCallContext(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()
	require.NoError(t, h.store.SetPendingCall(ctx, "latest", rstore.PendingCallInfo{OrgID: 999, IsInbound: true}))
	sess := NewCallSession("plivo-carrier-call", nil, zap.NewNop())
	sess.Provider, sess.CallSid, sess.LivePreparationSID = "plivo", "carrier-call", "plivo_in_3_call"
	sess.OrgID = 7
	frame := []byte(`{"event":"start","start":{"callId":"carrier-call","streamId":"incoming-stream"}}`)
	require.True(t, h.handleTextFrame(ctx, sess, frame), "missing trusted context must close the stream")
	require.EqualValues(t, 7, sess.OrgID)
	require.NoError(t, h.store.SetPendingCall(ctx, "carrier-call", rstore.PendingCallInfo{OrgID: 999, PlivoContextID: "plivo_in_3_call"}))
	require.True(t, h.handleTextFrame(ctx, sess, frame), "another org's context must close the stream")
}

func TestPlivoRejectsDuplicateCarrierStreams(t *testing.T) {
	h := newTestHandler(t)
	h.cfg.JWTSecret = "test-secret"
	ctx := context.Background()
	pending := rstore.PendingCallInfo{OrgID: 7, PlivoContextID: "plivo_in_3_call", ExotelCallSid: "carrier-call"}
	ticket, err := dial.PlivoStreamTicket(h.cfg.JWTSecret, "carrier-call", pending.PlivoContextID)
	require.NoError(t, err)
	for _, nonce := range []string{"first", "second"} {
		require.NoError(t, h.store.SetPendingCall(ctx, "plivo-ticket:"+nonce, pending))
	}
	server := httptest.NewServer(h)
	defer server.Close()
	conn := dialWS(t, server.URL+"/media-stream/plivo?ticket="+ticket+"&nonce=first")
	defer conn.Close()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/media-stream/plivo?ticket="+ticket+"&nonce=second", nil))
	require.Equal(t, 409, w.Code)
}
