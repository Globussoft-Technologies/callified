package dial

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type exotelControlTransport func(*http.Request) (*http.Response, error)

func (f exotelControlTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func exotelControlResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestExotelHangupUsesAccountHostAndAllLegs(t *testing.T) {
	for _, tc := range []struct{ region, subdomain, host string }{{"in", "", "api.in.exotel.com"}, {"", "", "api.exotel.com"}, {"in", "custom", "custom.exotel.com"}} {
		t.Run(tc.host, func(t *testing.T) {
			e := NewExotelClient("key", "token", "account", "", "", "", tc.region, tc.subdomain)
			puts := 0
			e.client.Transport = exotelControlTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, tc.host, r.URL.Host)
				user, pass, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "key", user)
				require.Equal(t, "token", pass)
				if r.Method == http.MethodGet {
					require.Equal(t, "/v1/Accounts/account/Calls/call/ActiveLegs.json", r.URL.Path)
					return exotelControlResponse(200, `{"Legs":[{"Sid":"leg1","CallSid":"call","AccountSid":"account"},{"Sid":"leg2"}]}`), nil
				}
				require.Equal(t, http.MethodPut, r.Method)
				require.Contains(t, []string{"/v1/Accounts/account/Calls/call/Legs/leg1", "/v1/Accounts/account/Calls/call/Legs/leg2"}, r.URL.Path)
				require.NoError(t, r.ParseForm())
				require.Equal(t, "hangup", r.Form.Get("Action"))
				require.Empty(t, r.Form.Get("Status"))
				puts++
				return exotelControlResponse(200, `{}`), nil
			})
			require.NoError(t, e.Hangup(context.Background(), "call"))
			require.Equal(t, 2, puts)
		})
	}
}

func TestExotelHangupHandlesProviderFailureAndEndedCalls(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		activeStatus           int
		activeBody, callStatus string
		success                bool
	}{
		{"forbidden", 403, `{}`, "in-progress", false},
		{"already ended", 404, `{}`, "completed", true},
		{"ringing not confirmed ended", 404, `{}`, "ringing", false},
		{"empty legs still active", 200, `{"Legs":[]}`, "in-progress", false},
		{"bad json", 200, `not json`, "completed", false},
		{"wrong call identity", 200, `{"Legs":[{"Sid":"leg","CallSid":"other"}]}`, "completed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExotelClient("key", "token", "account", "", "", "", "in", "")
			e.client.Transport = exotelControlTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				if strings.HasSuffix(r.URL.Path, "ActiveLegs.json") {
					return exotelControlResponse(tc.activeStatus, tc.activeBody), nil
				}
				return exotelControlResponse(200, `{"Call":{"Status":"`+tc.callStatus+`"}}`), nil
			})
			err := e.Hangup(context.Background(), "call")
			if tc.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
