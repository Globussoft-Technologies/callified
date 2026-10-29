package api

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTataDynamicEndpointRejectsNoSecretBeforeDatabase(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.tataStreamURL(w, httptest.NewRequest("POST", "/webhook/tata/stream-url", nil))
	require.Equal(t, 401, w.Code)
}
func TestTataSecretHashAndNumberValidation(t *testing.T) {
	require.Len(t, tataSecretHash("secret"), 64)
	require.NotEqual(t, "secret", tataSecretHash("secret"))
	require.True(t, sameTataNumber("+919999999999", "919999999999"))
	require.False(t, sameTataNumber("9999999999", "919999999999"))
	require.False(t, validTataNumber("+91abc"))
}

func TestAuthenticatedTataResolutionCanRetryBeforeUpgrade(t *testing.T) {
	m := miniredis.RunT(t)
	store := rstore.New("redis://"+m.Addr(), zap.NewNop())
	ctx := context.Background()
	pending := rstore.PendingCallInfo{OrgID: 9, ExotelAccountID: 7, Phone: "7795740488", CampaignID: 1}
	require.NoError(t, store.ReserveTataDial(ctx, &pending))
	server := &Server{store: store, cfg: &config.Config{PublicServerURL: "https://test.example"}}
	account := &db.OrgExotelAccount{ID: 7, OrgID: 9, Direction: "outbound", CallerID: "919240232665"}
	request := `{"callId":"carrier-a","fromNumber":"+919240232665","toNumber":"+917795740488","org_id":123}`
	resolve := func() string {
		w := httptest.NewRecorder()
		server.resolveTataStreamURL(ctx, w, httptest.NewRequest("POST", "/webhook/tata/stream-url", strings.NewReader(request)), account)
		require.Equal(t, 200, w.Code)
		var data map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
		require.Len(t, data, 2)
		require.Equal(t, true, data["success"])
		return data["wss_url"].(string)
	}
	first := resolve()
	require.Equal(t, first, resolve())
	parsed, err := url.Parse(first)
	require.NoError(t, err)
	require.Equal(t, "wss", parsed.Scheme)
	info, err := store.ConsumeTataStreamTicket(ctx, parsed.Query().Get("ticket"))
	require.NoError(t, err)
	require.Equal(t, int64(9), info.Pending.OrgID)
	require.Equal(t, int64(1), info.Pending.CampaignID)
	_, err = store.ConsumeTataStreamTicket(ctx, parsed.Query().Get("ticket"))
	require.Error(t, err)
}
func TestTataNotReadyIsConfigurationConflict(t *testing.T) {
	require.Equal(t, 409, dialErrorStatus(dial.ErrTataStreamNotReady))
}
