package api

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/globussoft/callified-backend/internal/db"
	plivo "github.com/plivo/plivo-go/v7"
	"github.com/stretchr/testify/require"
)

func TestForwardingActionOutcomes(t *testing.T) {
	for _, tc := range []struct {
		status, bleg, want string
		valid              bool
	}{
		{"completed", "leg-b", "answered", true},
		{"completed", "", "unknown", true},
		{"busy", "", "busy", true},
		{"no-answer", "", "no_answer", true},
		{"timeout", "", "no_answer", true},
		{"failed", "", "failed", true},
		{"cancel", "", "cancelled", true},
		{"ringing", "", "", false},
	} {
		t.Run(tc.status+tc.bleg, func(t *testing.T) {
			got, valid := forwardingActionOutcome(tc.status, tc.bleg)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.valid, valid)
		})
	}
}

func TestForwardingCallbackBindsParentCallAndAccount(t *testing.T) {
	account := &db.OrgExotelAccount{ID: 3, OrgID: 7, Provider: "plivo"}
	query := url.Values{"context_id": {"plivo_in_3_call-1"}}
	form := url.Values{"DialALegUUID": {"call-1"}, "DialStatus": {"busy"}}
	id, err := plivoForwardContext(account, query, form)
	require.NoError(t, err)
	require.Equal(t, "plivo_in_3_call-1", id)
	_, err = plivoForwardContext(&db.OrgExotelAccount{ID: 4, OrgID: 7, Provider: "plivo"}, query, form)
	require.Error(t, err)
	form.Set("DialALegUUID", "other-call")
	_, err = plivoForwardContext(account, query, form)
	require.Error(t, err)
	form.Set("DialALegUUID", "call-1")
	query.Add("context_id", "plivo_in_3_other")
	_, err = plivoForwardContext(account, query, form)
	require.Error(t, err)
}

func TestForwardingSignatureBindsOutcomeAndContext(t *testing.T) {
	const base = "https://calls.example"
	const path = "/webhook/plivo/inbound/dial-action?org_id=7&account_id=3&context_id=plivo_in_3_call-1"
	const token = "provider-token"
	params := map[string]string{"DialALegUUID": "call-1", "DialStatus": "busy"}
	r := httptest.NewRequest("POST", path, nil)
	r.Header.Set("X-Plivo-Signature-V3-Nonce", "nonce")
	r.Header.Set("X-Plivo-Signature-V3", plivo.ComputeSignatureV3(token, base+path, "POST", "nonce", params))
	require.True(t, validPlivoSignature(r, base, token, params))
	require.False(t, validPlivoSignature(r, base, "wrong", params))
	require.False(t, validPlivoSignature(r, base, token, map[string]string{"DialALegUUID": "call-1", "DialStatus": "completed"}))
	r.URL.RawQuery = "org_id=8&account_id=3&context_id=plivo_in_3_call-1"
	require.False(t, validPlivoSignature(r, base, token, params))
}
