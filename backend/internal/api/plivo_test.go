package api

import (
	"context"
	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"go.uber.org/zap"
	"net/http/httptest"
	"testing"

	plivo "github.com/plivo/plivo-go/v7"
)

func TestPlivoAnswerBindsBeforeDialResponse(t *testing.T) {
	ctx := context.Background()
	store := rstore.New("", zap.NewNop())
	account := &db.OrgExotelAccount{ID: 3, OrgID: 2}
	pending := rstore.PendingCallInfo{PlivoContextID: "app-call", OrgID: 2, ExotelAccountID: 3, LeadID: 9}
	if err := store.SetPendingCall(ctx, "app-call", pending); err != nil {
		t.Fatal(err)
	}
	// No request UUID alias exists yet: the dial HTTP request is still in flight.
	bound, err := bindPlivoCall(ctx, store, account, "app-call", "carrier-call", "carrier-request")
	if err != nil {
		t.Fatal(err)
	}
	if bound.PlivoContextID != "app-call" || bound.PlivoRequestID != "carrier-request" || bound.ExotelCallSid != "carrier-call" {
		t.Fatalf("wrong binding: %+v", bound)
	}
	for _, key := range []string{"app-call", "carrier-call"} {
		got, ok := store.GetPendingCall(ctx, key)
		if !ok || got.LeadID != 9 || got.ExotelCallSid != "carrier-call" {
			t.Fatalf("missing identity at %s", key)
		}
	}
	if _, err := bindPlivoCall(ctx, store, account, "app-call", "different-call", "carrier-request"); err == nil {
		t.Fatal("changed carrier identity accepted")
	}
	if _, err := bindPlivoCall(ctx, store, &db.OrgExotelAccount{ID: 3, OrgID: 8}, "app-call", "carrier-call", "carrier-request"); err == nil {
		t.Fatal("cross-tenant context accepted")
	}
}

func TestPlivoSignatureBindsURLAndBody(t *testing.T) {
	const base = "https://calls.example.com"
	const path = "/webhook/plivo/answer?org_id=2&account_id=3"
	const token = "test-auth-token"
	params := map[string]string{"CallUUID": "call-1", "RequestUUID": "request-1"}
	r := httptest.NewRequest("POST", path, nil)
	r.Header.Set("X-Plivo-Signature-V3-Nonce", "12345")
	r.Header.Set("X-Plivo-Signature-V3", plivo.ComputeSignatureV3(token, base+path, "POST", "12345", params))
	if !validPlivoSignature(r, base, token, params) {
		t.Fatal("valid signed webhook rejected")
	}
	if validPlivoSignature(r, base, "wrong-token", params) {
		t.Fatal("wrong credential accepted")
	}
	if validPlivoSignature(r, base, token, map[string]string{"CallUUID": "other-call"}) {
		t.Fatal("modified body accepted")
	}
	r.URL.RawQuery = "org_id=4&account_id=3"
	if validPlivoSignature(r, base, token, params) {
		t.Fatal("modified tenant accepted")
	}
	r.URL.RawQuery = "org_id=2&account_id=3"
	r.Header.Del("X-Plivo-Signature-V3-Nonce")
	if validPlivoSignature(r, base, token, params) {
		t.Fatal("missing nonce accepted")
	}
}
