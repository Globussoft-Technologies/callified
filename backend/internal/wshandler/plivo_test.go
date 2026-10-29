package wshandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/globussoft/callified-backend/internal/dial"
)

func TestRecordingCallIdentity(t *testing.T) {
	for _, tc := range []struct{ provider, carrier, app, want string }{
		{"plivo", "actual-carrier-uuid", "app-call", "app-call"},
		{"exotel", "exotel-sid", "", "exotel-sid"},
		{"tata", "tata-sid", "", "tata-sid"},
	} {
		sess := &CallSession{Provider: tc.provider, CallSid: tc.carrier, LivePreparationSID: tc.app}
		if got := recordingCallSID(sess); got != tc.want {
			t.Fatalf("%s recording identity: %s", tc.provider, got)
		}
	}
}

func TestPlivoMediaRejectsMissingOrConsumedTicket(t *testing.T) {
	h := newTestHandler(t)
	h.cfg.JWTSecret = "plivo-test-secret"
	for _, path := range []string{"/media-stream/plivo", "/media-stream/plivo?ticket=invalid"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	}
	ticket, err := dial.PlivoStreamTicket(h.cfg.JWTSecret, "call-1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/media-stream/plivo?ticket="+ticket+"&nonce=already-consumed", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("consumed ticket accepted: %d", w.Code)
	}
}

func TestPlivoStartRejectsDifferentCall(t *testing.T) {
	h := newTestHandler(t)
	sess := &CallSession{Provider: "plivo", CallSid: "expected-call"}
	for _, frame := range []string{`{"event":"start","start":{"callId":"other-call"}}`, `{"event":"start"}`} {
		if !h.handleTextFrame(context.Background(), sess, []byte(frame)) {
			t.Fatal("mismatched start did not stop the connection")
		}
	}
}
