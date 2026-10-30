package wshandler

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http/httptest"
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
