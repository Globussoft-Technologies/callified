package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBrowserCallTicketRequiresCallOwnerAndOrg(t *testing.T) {
	owner := AuthClaims{Email: "owner@example.com", OrgID: 7, Role: "Admin", UserID: 42}
	log := zap.NewNop()
	s := &Server{cfg: &config.Config{JWTSecret: "secret", SuperAdminEmail: owner.Email}, store: rstore.New("", log), logger: log}
	for _, tc := range []struct {
		name    string
		pending rstore.PendingCallInfo
		want    int
	}{
		{"owner", rstore.PendingCallInfo{IsBridge: true, OrgID: 7, UserEmail: owner.Email}, http.StatusOK},
		{"other org", rstore.PendingCallInfo{IsBridge: true, OrgID: 8, UserEmail: owner.Email}, http.StatusNotFound},
		{"other admin", rstore.PendingCallInfo{IsBridge: true, OrgID: 7, UserEmail: "other@example.com"}, http.StatusNotFound},
		{"AI call", rstore.PendingCallInfo{IsBridge: false, OrgID: 7, UserEmail: owner.Email}, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, s.store.SetPendingCall(context.Background(), "app-call", tc.pending))
			r := httptest.NewRequest("GET", "/api/calls/app-call/agent-ticket", nil)
			r.SetPathValue("call_sid", "app-call")
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, owner))
			w := httptest.NewRecorder()
			s.agentTicket(w, r)
			require.Equal(t, tc.want, w.Code)
			if tc.want == http.StatusOK {
				var body struct {
					Ticket string `json:"ticket"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				claims, err := dial.VerifyAgentTicket("secret", body.Ticket, "app-call")
				require.NoError(t, err)
				require.EqualValues(t, owner.OrgID, claims.OrgID)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			}
			// The same owner boundary protects hangup, even before dialing.
			r.Method = "DELETE"
			w = httptest.NewRecorder()
			s.hangupBrowserCall(w, r)
			if tc.want == http.StatusOK {
				require.Equal(t, http.StatusServiceUnavailable, w.Code)
			} else {
				require.Equal(t, http.StatusNotFound, w.Code)
			}
		})
	}
}
