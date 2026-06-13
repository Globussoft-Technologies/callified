package conversation

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/globussoft/callified-backend/internal/receptionist/ambulance"
	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/llm"
	"github.com/globussoft/callified-backend/internal/receptionist/models"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

// Skip the artificial reply delay in tests — config.Get() reads the
// env on first call, so setting this before any test runs is enough.
func init() {
	os.Setenv("MIN_TURN_LATENCY_MS", "0")
}

// newTestManager constructs a Manager wired with fresh services and an
// LLM agent that's guaranteed disabled (no API key) so we exercise the
// rule-based FSM path.
func newTestManager() *Manager {
	store := session.New(30 * time.Minute)
	apptSvc := appointment.New()
	ambSvc := ambulance.New()
	llmAgent := llm.New(apptSvc, ambSvc) // Enabled() == false without API key
	return New(store, apptSvc, ambSvc, llmAgent)
}

func TestFullBookingFlow(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550100", "", "")

	turns := []string{
		"Hi, my name is Alice Walker",
		"I'd like to book an appointment",
		"Dr. John",
		"Tomorrow at 10am",
	}
	var last *TurnResult
	for _, t := range turns {
		last = m.HandleTurn(sess.ID, t)
	}
	if last.State != models.StateAwaitingFollowup {
		t.Errorf("state = %s, want awaiting_followup", last.State)
	}
	id, _ := last.Metadata["appointment_id"].(string)
	if !strings.HasPrefix(id, "APT-") {
		t.Errorf("appointment_id = %q, want APT- prefix", id)
	}
}

func TestEmergencyShortCircuit(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550101", "", "")
	m.HandleTurn(sess.ID, "My name is Bob")
	res := m.HandleTurn(sess.ID, "I have severe chest pain")

	if !res.IsEmergency {
		t.Fatal("expected emergency")
	}
	if res.Intent != models.IntentEmergency {
		t.Errorf("intent = %s, want emergency", res.Intent)
	}
	if res.State != models.StateEmergency {
		t.Errorf("state = %s, want emergency", res.State)
	}
	if !strings.Contains(res.Message, "911") {
		t.Errorf("message missing 911 instruction")
	}
	id, _ := res.Metadata["dispatch_id"].(string)
	if !strings.HasPrefix(id, "AMB-") {
		t.Errorf("dispatch_id = %q, want AMB- prefix", id)
	}
}

func TestEmergencyAtFirstTurn(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550102", "", "")
	res := m.HandleTurn(sess.ID, "He's unconscious!")
	if !res.IsEmergency {
		t.Fatal("expected emergency on first turn")
	}
}

func TestEmergencyDispatchIdempotent(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550199", "", "")
	r1 := m.HandleTurn(sess.ID, "He's unconscious!")
	r2 := m.HandleTurn(sess.ID, "Please send help, this is an emergency")
	id1, _ := r1.Metadata["dispatch_id"].(string)
	id2, _ := r2.Metadata["dispatch_id"].(string)
	if id1 == "" || id1 != id2 {
		t.Errorf("dispatch ids differ across emergency turns: %q vs %q", id1, id2)
	}
	if !strings.Contains(strings.ToLower(r2.Message), "already") {
		t.Errorf("expected reaffirmation in message: %q", r2.Message)
	}
}

func TestEmergencyAddressCapture(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550110", "", "")
	r1 := m.HandleTurn(sess.ID, "I have chest pain")
	if !r1.IsEmergency {
		t.Fatal("expected emergency")
	}
	addr := "742 Evergreen Terrace"
	r2 := m.HandleTurn(sess.ID, addr)
	if r2.IsEmergency {
		t.Error("address-capture turn should not re-flag is_emergency")
	}
	if loc, _ := r2.Metadata["location"].(string); loc != addr {
		t.Errorf("location metadata = %q, want %q", loc, addr)
	}
	// After capturing the address the session should release back to
	// normal flow so the caller can ask follow-up questions.
	if r2.State != models.StateAwaitingFollowup {
		t.Errorf("state = %s, want awaiting_followup", r2.State)
	}
	r3 := m.HandleTurn(sess.ID, "What are your hours?")
	if r3.Intent != models.IntentInquiryHours {
		t.Errorf("after-emergency follow-up intent = %s, want inquiry_hours", r3.Intent)
	}
}

func TestInquiryHours(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550103", "", "")
	m.HandleTurn(sess.ID, "My name is Carol")
	res := m.HandleTurn(sess.ID, "What are your hours?")
	if res.Intent != models.IntentInquiryHours {
		t.Errorf("intent = %s, want inquiry_hours", res.Intent)
	}
}

func TestCancelFlow(t *testing.T) {
	m := newTestManager()
	sess, _ := m.StartCall("+15555550104", "", "")
	m.HandleTurn(sess.ID, "I'm Dave")
	booking := m.HandleTurn(sess.ID, "Book me with Dr. Elina tomorrow at 2pm")
	id, _ := booking.Metadata["appointment_id"].(string)
	if id == "" {
		t.Fatalf("booking did not return an appointment id; message=%q", booking.Message)
	}
	cancel := m.HandleTurn(sess.ID, "Cancel my appointment "+id)
	if status, _ := cancel.Metadata["status"].(string); status != "cancelled" {
		t.Errorf("status = %q, want cancelled", status)
	}
}

func TestSessionNotFoundReturnsNil(t *testing.T) {
	m := newTestManager()
	if got := m.HandleTurn("does-not-exist", "hello"); got != nil {
		t.Errorf("expected nil for unknown session, got %+v", got)
	}
}
