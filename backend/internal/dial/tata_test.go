package dial

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTataHangupUsesInitiationRefID(t *testing.T) {
	var gotAuth string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/call/hangup", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Call hangup successful"})
	}))
	defer server.Close()

	client := NewTataClient("support-key", "test-token", "caller", "agent", server.URL+"/v1/click_to_call_support")
	require.NoError(t, client.Hangup(context.Background(), "ref-123"))
	assert.Equal(t, "Bearer test-token", gotAuth)
	assert.Equal(t, map[string]string{"ref_id": "ref-123"}, gotBody)
}

func TestTataHangupRetriesRawTokenAfterBearerRejection(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") == "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, "test-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	client := NewTataClient("support-key", "test-token", "caller", "agent", server.URL+"/v1/click_to_call_support")
	require.NoError(t, client.Hangup(context.Background(), "ref-123"))
	assert.Equal(t, 2, requests)
}

func TestTataHangupRejectsUnsuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Invalid Ref ID"})
	}))
	defer server.Close()

	client := NewTataClient("support-key", "Bearer test-token", "caller", "agent", server.URL)
	err := client.Hangup(context.Background(), "bad-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Ref ID")
}

func TestTataInitiateUsesDistinctCredentials(t *testing.T) {
	for _, token := range []string{"account-token", "Bearer account-token", "bearer account-token"} {
		t.Run(token, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, "Bearer account-token", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "support-key", body["api_key"])
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"ref_id":"call-123"}`))
			}))
			defer server.Close()
			client := NewTataClient("support-key", token, "918011111111", "9999999999", server.URL+"/v1/click_to_call_support")
			id, err := client.InitiateCall(context.Background(), "9876543210", "", "")
			require.NoError(t, err)
			assert.Equal(t, "call-123", id)
			assert.Equal(t, 1, requests)
		})
	}
}
func TestTataInitiateRejectsMissingCredentials(t *testing.T) {
	for _, credentials := range [][2]string{{"support-key", ""}, {"", "account-token"}} {
		client := NewTataClient(credentials[0], credentials[1], "caller", "agent", "http://127.0.0.1:1")
		_, err := client.InitiateCall(context.Background(), "9876543210", "", "")
		require.ErrorContains(t, err, "Smartflo API token")
	}
}
