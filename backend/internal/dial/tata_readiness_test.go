package dial

import (
	"testing"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/require"
)

func TestTataAccountReadiness(t *testing.T) {
	for _, provider := range []string{"tata", "smartflo", "tata_tele"} {
		t.Run(provider, func(t *testing.T) {
			creds := db.ExotelCreds{Provider: provider, APIKey: "support-key", CallerID: "number", AppID: "agent"}
			require.False(t, creds.IsSet())
			require.ErrorContains(t, validateTataAccountReadiness(creds), "Smartflo API token")
			creds.APIToken = "  "
			require.False(t, creds.IsSet())
			require.Error(t, validateTataAccountReadiness(creds))
			creds.APIToken = "account-token"
			require.True(t, creds.IsSet())
			require.NoError(t, validateTataAccountReadiness(creds))
			creds.Direction, creds.APIToken = "inbound", ""
			require.True(t, creds.IsSet())
			require.NoError(t, validateTataAccountReadiness(creds))
		})
	}
	require.NoError(t, validateTataAccountReadiness(db.ExotelCreds{Provider: "exotel"}))
}
