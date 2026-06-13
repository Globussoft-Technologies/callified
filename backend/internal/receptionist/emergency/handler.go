// Package emergency detects medical-emergency keywords in caller speech.
//
// This is a hard guardrail that runs before any LLM call. We deliberately
// keep it rule-based: false positives are far less harmful than false
// negatives on a real medical emergency, so the matcher is broad and
// explicit. The package has no dependency on conversation state — any
// caller, in any state, can trip it.
package emergency

import (
	"log"
	"regexp"
	"strings"
)

// Detection is the result of one match check.
type Detection struct {
	IsEmergency   bool
	MatchedPhrase string
}

// Response is the canned reply when no ambulance dispatch is configured.
const Response = "This sounds like an emergency. Please hang up and call 911 " +
	"immediately, or your local emergency number. If you are with the " +
	"patient, stay on the line with emergency services and follow their " +
	"instructions. I am notifying our on-call staff right now."

// emergencyPhrases is the union of regex fragments that indicate a
// likely medical emergency. Compiled once at init.
var emergencyPhrases = []string{
	// Cardiac / respiratory
	`chest pain`, `chest pressure`, `heart attack`,
	`can(?:'|no)t breathe`, `cannot breathe`,
	`difficulty breathing`, `trouble breathing`,
	`shortness of breath`, `struggling to breathe`, `choking`,
	// Neurological
	`stroke`, `unconscious`, `passed out`, `not responding`,
	`unresponsive`, `seizure`, `convulsions`,
	// Bleeding / trauma
	`bleeding heavily`, `heavy bleeding`,
	`won(?:'|no)t stop bleeding`, `severe bleeding`,
	`hemorrhag(?:e|ing)`, `head injury`, `broken bone`, `compound fracture`,
	// Toxicology / overdose
	`overdos(?:e|ed|ing)`, `poisoned`,
	`swallowed (?:bleach|poison|pills)`,
	// Mental health crisis
	`suicide`, `suicidal`, `kill (?:my|him|her)self`, `want to die`,
	// Generic distress
	`emergency`, `call (?:911|nine one one)`, `dying`,
	`life threatening`, `anaphyla(?:xis|ctic)`,
	`severe allergic reaction`,
}

var emergencyRE = regexp.MustCompile(
	`(?i)\b(?:` + strings.Join(emergencyPhrases, "|") + `)\b`,
)

// Detect returns whether the utterance contains an emergency keyword.
// Case-insensitive; matches on word boundaries.
func Detect(text string) Detection {
	if text == "" {
		return Detection{}
	}
	if m := emergencyRE.FindString(text); m != "" {
		log.Printf("emergency keyword detected: %q", m)
		return Detection{IsEmergency: true, MatchedPhrase: m}
	}
	return Detection{}
}
