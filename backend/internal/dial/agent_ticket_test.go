package dial

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestAgentTicketScope(t *testing.T) {
	ticket, err := AgentTicket("secret", "call-1", 7, 9)
	require.NoError(t, err)
	claims, err := VerifyAgentTicket("secret", ticket, "call-1")
	require.NoError(t, err)
	require.EqualValues(t, 7, claims.OrgID)
	require.EqualValues(t, 9, claims.UserID)
	_, err = VerifyAgentTicket("secret", ticket, "call-2")
	require.Error(t, err)
	_, err = VerifyAgentTicket("wrong", ticket, "call-1")
	require.Error(t, err)
	for _, bad := range []jwt.MapClaims{
		{"sub": "call-1", "org_id": 7, "user_id": 9, "kind": "monitor", "exp": time.Now().Add(time.Minute).Unix()},
		{"sub": "call-1", "org_id": 7, "user_id": 9, "kind": "agent", "exp": time.Now().Add(-time.Minute).Unix()},
		{"sub": "call-1", "org_id": 7, "user_id": 9, "kind": "agent"},
	} {
		invalid, err := jwt.NewWithClaims(jwt.SigningMethodHS256, bad).SignedString([]byte("secret"))
		require.NoError(t, err)
		_, err = VerifyAgentTicket("secret", invalid, "call-1")
		require.Error(t, err)
	}
	// Admin calls intentionally use UserID=0 to retain campaign credentials.
	adminTicket, err := AgentTicket("secret", "admin-call", 7, 0)
	require.NoError(t, err)
	_, err = VerifyAgentTicket("secret", adminTicket, "admin-call")
	require.NoError(t, err)
}
