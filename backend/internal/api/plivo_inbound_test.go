package api

import (
	"context"
	"encoding/xml"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPlivoIncomingNumberAndIdentity(t *testing.T) {
	account := &db.OrgExotelAccount{ID: 3, OrgID: 7, Provider: "plivo", CallerID: "80 1234 5678", Direction: "outbound"}
	form := url.Values{"Direction": {"inbound"}, "CallUUID": {"carrier-call"}, "To": {"+918012345678"}, "From": {"+919123456789"}, "org_id": {"999"}}
	contextID, phone, err := plivoInboundIdentity(account, form)
	require.NoError(t, err, "the saved outbound account can also receive calls")
	require.Equal(t, "plivo_in_3_carrier-call", contextID)
	require.Equal(t, "+919123456789", phone)
	for _, tc := range []struct{ key, value string }{
		{"Direction", "outbound"}, {"To", "+918011111111"}, {"To", ""}, {"CallUUID", ""}, {"CallUUID", "../../other"},
	} {
		copy := make(url.Values)
		for key, value := range form {
			copy[key] = append([]string(nil), value...)
		}
		copy.Set(tc.key, tc.value)
		_, _, err := plivoInboundIdentity(account, copy)
		require.Error(t, err, "%s=%s", tc.key, tc.value)
	}
	form.Set("From", "anonymous")
	_, phone, err = plivoInboundIdentity(account, form)
	require.NoError(t, err)
	require.Empty(t, phone, "unknown caller is not invented from another tenant's lead")
}

func TestPlivoIncomingStreamCarriesVerifiedContext(t *testing.T) {
	store := rstore.New("", zap.NewNop())
	s := &Server{cfg: &config.Config{JWTSecret: "test-secret", PublicServerURL: "https://calls.example.com"}, store: store}
	pending := rstore.PendingCallInfo{IsInbound: true, OrgID: 7, ExotelAccountID: 3, ExotelCallSid: "carrier-call", PlivoContextID: "plivo_in_3_carrier-call"}
	w := httptest.NewRecorder()
	s.writePlivoStream(w, httptest.NewRequest("POST", "/webhook/plivo/inbound", nil), "carrier-call", pending.PlivoContextID, pending)
	require.Equal(t, 200, w.Code)
	var body struct {
		Stream struct {
			ContentType   string `xml:"contentType,attr"`
			Bidirectional string `xml:"bidirectional,attr"`
			URL           string `xml:",chardata"`
		} `xml:"Stream"`
	}
	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "audio/x-mulaw;rate=8000", body.Stream.ContentType)
	require.Equal(t, "true", body.Stream.Bidirectional)
	stream, err := url.Parse(body.Stream.URL)
	require.NoError(t, err)
	require.Equal(t, "wss", stream.Scheme)
	call, identity, err := dial.VerifyPlivoStreamTicket("test-secret", stream.Query().Get("ticket"))
	require.NoError(t, err)
	require.Equal(t, "carrier-call", call)
	require.Equal(t, pending.PlivoContextID, identity)
	got, found := store.TakePendingCall(context.Background(), "plivo-ticket:"+stream.Query().Get("nonce"))
	require.True(t, found)
	require.Equal(t, pending, got)
	_, found = store.TakePendingCall(context.Background(), "plivo-ticket:"+stream.Query().Get("nonce"))
	require.False(t, found, "media ticket is single-use")
}

func TestIncomingPlivoAccountValidationAndHTTPS(t *testing.T) {
	require.Empty(t, validateProviderAccount("plivo", "inbound", "Receptionist", "", "token", "", "auth-id", "+918012345678", ""))
	require.NotEmpty(t, validateProviderAccount("plivo", "inbound", "Receptionist", "", "", "", "auth-id", "+918012345678", ""))
	for _, raw := range []string{"http://example.com", "https://user:password@example.com", "https://example.com?secret=x", ""} {
		require.False(t, validPlivoPublicURL(raw))
	}
	require.True(t, validPlivoPublicURL("https://calls.example.com"))
	s := &Server{cfg: &config.Config{PublicServerURL: "http://example.com"}}
	w := httptest.NewRecorder()
	s.writePlivoStream(w, httptest.NewRequest("POST", "/", strings.NewReader("")), "call", "context", rstore.PendingCallInfo{})
	require.Equal(t, 503, w.Code)
}
