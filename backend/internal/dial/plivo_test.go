package dial

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlivoOutboundAndHangup(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, token, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "auth-id", id)
		require.Equal(t, "auth-token", token)
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == "POST" {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "https://example.com/answer", body["answer_url"])
			require.Equal(t, "POST", body["answer_method"])
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"request_uuid":"request-123"}`))
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	client := NewPlivoClient("auth-id", "auth-token")
	client.baseURL = server.URL + "/v1/Account/"
	id, err := client.InitiateCall(context.Background(), "+919123456789", "+919876543210", "https://example.com/answer", "https://example.com/status")
	require.NoError(t, err)
	require.Equal(t, "request-123", id)
	require.NoError(t, client.Hangup(context.Background(), "call-123"))
	require.NoError(t, client.CancelRequest(context.Background(), id))
	require.Equal(t, []string{"POST /v1/Account/auth-id/Call/", "DELETE /v1/Account/auth-id/Call/call-123/", "DELETE /v1/Account/auth-id/Request/request-123/"}, paths)
}

func TestPlivoDoesNotLeakProviderErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("secret-auth-token"))
	}))
	defer server.Close()
	client := NewPlivoClient("id", "secret-auth-token")
	client.baseURL = server.URL + "/"
	_, err := client.InitiateCall(context.Background(), "a", "b", "c", "d")
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "secret-auth-token"))
}

func TestPlivoStreamTicketScope(t *testing.T) {
	token, err := PlivoStreamTicket("secret", "call", "request")
	require.NoError(t, err)
	call, request, err := VerifyPlivoStreamTicket("secret", token)
	require.NoError(t, err)
	require.Equal(t, "call", call)
	require.Equal(t, "request", request)
	_, _, err = VerifyPlivoStreamTicket("wrong", token)
	require.Error(t, err)
	_, _, err = VerifyPlivoStreamTicket("secret", "invalid")
	require.Error(t, err)
}
