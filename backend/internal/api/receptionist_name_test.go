package api

import (
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTranscriptDoesNotReplaceCRMName(t *testing.T) {
	for _, name := range []string{"Existing", ""} {
		call := db.InboundReceptionistCall{FirstName: name, Phone: "9876543210", Transcript: []byte(`[{"role":"User","text":"My name is Different"}]`)}
		fillReceptionistCallFromTranscript(&call)
		require.Equal(t, name, call.FirstName)
	}
}
