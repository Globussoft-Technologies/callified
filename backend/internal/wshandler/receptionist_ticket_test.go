package wshandler

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/dial"
	"github.com/stretchr/testify/require"
)

func TestReceptionistSimulatorRequiresTenantTicket(t *testing.T) {
	h := &Handler{cfg: &config.Config{JWTSecret: "secret"}}
	ticket, err := dial.ReceptionistTicket("secret", "web_sim_inbound_123", 7, 9)
	require.NoError(t, err)
	for _, query := range []url.Values{
		{"mode": {"inbound-sim"}, "org_id": {"7"}, "stream_sid": {"web_sim_inbound_123"}},
		{"mode": {"inbound-sim"}, "org_id": {"8"}, "stream_sid": {"web_sim_inbound_123"}, "ticket": {ticket}},
		{"mode": {"inbound-sim"}, "org_id": {"7"}, "stream_sid": {"other"}, "ticket": {ticket}},
		{"direction": {"inbound"}, "org_id": {"7"}},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/media-stream?"+query.Encode(), nil))
		require.Equal(t, 403, w.Code)
	}
}
