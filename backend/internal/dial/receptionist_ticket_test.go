package dial

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReceptionistTicketBoundToStreamAndKind(t *testing.T) {
	ticket, err := ReceptionistTicket("secret", "web_sim_inbound_123", 7, 9)
	require.NoError(t, err)
	claims, err := VerifyReceptionistTicket("secret", ticket, "web_sim_inbound_123")
	require.NoError(t, err)
	require.EqualValues(t, 7, claims.OrgID)
	require.EqualValues(t, 9, claims.UserID)
	_, err = VerifyReceptionistTicket("secret", ticket, "web_sim_inbound_other")
	require.Error(t, err)
	_, err = VerifyReceptionistTicket("wrong", ticket, "web_sim_inbound_123")
	require.Error(t, err)
	agent, err := AgentTicket("secret", "web_sim_inbound_123", 7, 9)
	require.NoError(t, err)
	_, err = VerifyReceptionistTicket("secret", agent, "web_sim_inbound_123")
	require.Error(t, err)
}
