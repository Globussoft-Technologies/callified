package intent

import (
	"testing"

	"github.com/globussoft/callified-backend/internal/receptionist/models"
)

func TestIntentRouting(t *testing.T) {
	cases := map[string]models.IntentType{
		"Hi there":                            models.IntentGreeting,
		"I'd like to book an appointment":     models.IntentBookAppointment,
		"Can I schedule a checkup?":           models.IntentBookAppointment,
		"I want to reschedule my appointment": models.IntentRescheduleAppointment,
		"Cancel my appointment please":        models.IntentCancelAppointment,
		"What are your hours?":                models.IntentInquiryHours,
		"Where are you located?":              models.IntentInquiryLocation,
		"Which doctors are available?":        models.IntentInquiryDoctor,
		"Goodbye":                             models.IntentGoodbye,
		"Yes please":                          models.IntentAffirm,
		"No thanks":                           models.IntentDeny,
	}
	for text, want := range cases {
		got := Detect(text).Intent
		if got != want {
			t.Errorf("Detect(%q).Intent = %s, want %s", text, got, want)
		}
	}
}

func TestExtractName(t *testing.T) {
	if got := ExtractName("My name is Jane Smith"); got != "Jane Smith" {
		t.Errorf("got %q want Jane Smith", got)
	}
	if got := ExtractName("I'm Alex"); got != "Alex" {
		t.Errorf("got %q want Alex", got)
	}
	if got := ExtractName("Mary Johnson"); got != "Mary Johnson" {
		t.Errorf("got %q want Mary Johnson", got)
	}
}

func TestExtractSlots(t *testing.T) {
	s := ExtractSlots("I'd like to see Dr. Patel tomorrow at 10am")
	if s["doctor"] != "Patel" {
		t.Errorf("doctor = %q, want Patel", s["doctor"])
	}
	if s["date"] != "tomorrow" {
		t.Errorf("date = %q, want tomorrow", s["date"])
	}
	if s["time"] != "10am" {
		t.Errorf("time = %q, want 10am", s["time"])
	}

	s2 := ExtractSlots("My appointment id is APT-ABC123")
	if s2["appointment_id"] != "APT-ABC123" {
		t.Errorf("appointment_id = %q, want APT-ABC123", s2["appointment_id"])
	}
}

func TestDoctorRegexRejectsCommonWords(t *testing.T) {
	// "doctor in your hospital" must NOT extract "in" as a doctor name.
	s := ExtractSlots("can you suggest the names who is working on as a doctor in your hospital")
	if v, ok := s["doctor"]; ok {
		t.Errorf("expected no doctor extracted, got %q", v)
	}
}
