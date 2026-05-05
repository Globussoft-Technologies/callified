package emergency

import "testing"

func TestEmergencyPhrasesDetected(t *testing.T) {
	cases := []string{
		"I have severe chest pain",
		"My father is unconscious",
		"She's bleeding heavily and won't stop",
		"I can't breathe",
		"I think I'm having a heart attack",
		"He had a seizure",
		"She's having a stroke",
		"I overdosed on pills",
		"This is an emergency",
		"Please call 911",
		"I'm having difficulty breathing",
		"I want to kill myself",
		"Anaphylactic shock",
	}
	for _, c := range cases {
		if !Detect(c).IsEmergency {
			t.Errorf("expected emergency match for %q", c)
		}
	}
}

func TestNonEmergencyNotFlagged(t *testing.T) {
	cases := []string{
		"I'd like to book an appointment",
		"What are your hours?",
		"Can I reschedule my visit?",
		"My name is Alex",
		"",
		"I have a mild headache and would like to see a doctor",
	}
	for _, c := range cases {
		if Detect(c).IsEmergency {
			t.Errorf("did not expect emergency match for %q", c)
		}
	}
}

func TestMatchedPhraseReturned(t *testing.T) {
	d := Detect("She has chest pain and is sweating")
	if !d.IsEmergency {
		t.Fatal("expected emergency")
	}
	if d.MatchedPhrase == "" {
		t.Fatal("expected matched phrase")
	}
}
