// Package intent classifies caller utterances and extracts slot values
// (doctor name, date, time, appointment id, patient name).
//
// Rule-based on purpose: predictable, debuggable, microsecond-fast for
// a small domain. The LLM path replaces this entirely when configured;
// see internal/llm.
package intent

import (
	"regexp"
	"strings"

	"github.com/globussoft/callified-backend/internal/receptionist/models"
)

// Match is one classification result.
type Match struct {
	Intent     models.IntentType
	Confidence float64
	Slots      map[string]string
}

type rule struct {
	intent   models.IntentType
	priority int // lower = higher priority on ties
	patterns []*regexp.Regexp
}

func mustCompileAll(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(`(?i)` + p)
	}
	return out
}

var rules = []rule{
	{models.IntentGreeting, 50, mustCompileAll(
		`\b(?:hi|hello|hey|good\s+(?:morning|afternoon|evening))\b`,
	)},
	{models.IntentGoodbye, 50, mustCompileAll(
		`\b(?:bye|goodbye|that'?s all|nothing else|thank you,?\s*bye)\b`,
		`\bend\s+(?:the\s+)?call\b`,
	)},
	{models.IntentAffirm, 60, mustCompileAll(
		`^\s*(?:yes|yeah|yep|yup|sure|correct|that'?s right|please do|sounds good|ok(?:ay)?)\b`,
	)},
	{models.IntentDeny, 60, mustCompileAll(
		`^\s*(?:no|nope|not really|don'?t|never mind|cancel that)\b`,
	)},
	{models.IntentBookAppointment, 10, mustCompileAll(
		`\bbook\b.*\b(?:appointment|visit|slot|checkup|check-up|dr\.?|doctor)\b`,
		`\bbook\s+(?:me|us|him|her)\b`,
		`\bschedule\b.*\b(?:appointment|visit|checkup|check-up|dr\.?|doctor)\b`,
		`\bmake\s+an\s+appointment\b`,
		`\bset\s+up\s+(?:an\s+)?appointment\b`,
		`\bsee\s+(?:a\s+|the\s+)?doctor\b`,
		`\bnew\s+appointment\b`,
	)},
	{models.IntentRescheduleAppointment, 10, mustCompileAll(
		`\breschedul\w*\b`,
		`\bmove\b.*\bappointment\b`,
		`\bchange\b.*\bappointment\b`,
		`\bdifferent\s+(?:time|day|date)\b.*\bappointment\b`,
	)},
	{models.IntentCancelAppointment, 10, mustCompileAll(
		`\bcancel\b.*\bappointment\b`,
		`\bcancel\s+my\s+(?:visit|booking|slot)\b`,
	)},
	{models.IntentInquiryHours, 20, mustCompileAll(
		`\b(?:open|opening|close|closing|hours)\b`,
		`\bwhen\s+(?:are\s+you|do\s+you)\s+open\b`,
		`\bwhat\s+time\b.*\b(?:open|close)\b`,
	)},
	{models.IntentInquiryLocation, 20, mustCompileAll(
		`\bwhere\s+are\s+you\b`,
		`\b(?:address|location|directions)\b`,
		`\bhow\s+do\s+I\s+get\s+(?:there|to\s+the\s+clinic)\b`,
	)},
	{models.IntentInquiryDoctor, 20, mustCompileAll(
		`\b(?:which|what)\s+doctor(?:s)?\b`,
		`\bwho\s+(?:is|are)\s+(?:available|the\s+doctors?|working)\b`,
		`\bdoctor'?s?\s+(?:available|availability|on\s+today)\b`,
		`\bany\s+doctor(?:s)?\s+available\b`,
		`\b(?:list|name|names)\s+(?:of\s+)?(?:the\s+)?doctor(?:s)?\b`,
		`\bsuggest\b.*\b(?:doctor|name|names|hospital|clinic)\b`,
		`\bdoctor(?:s)?\s+(?:do\s+you\s+have|you\s+have|in\s+(?:your|the)\s+(?:hospital|clinic))\b`,
		`\btell\s+me\s+(?:about\s+)?(?:the\s+)?doctor(?:s)?\b`,
	)},
}

// --- Slot extractors ---------------------------------------------------

var (
	doctorRE = regexp.MustCompile(`(?i)\b(?:dr\.?|doctor)\s+([A-Za-z][a-zA-Z'\-]+)`)
	apptRE   = regexp.MustCompile(`(?i)\b(APT-[A-Z0-9]{4,10})\b`)
	dateRE   = regexp.MustCompile(`(?i)\b(today|tomorrow|` +
		`(?:mon|tues|wednes|thurs|fri|satur|sun)day|` +
		`\d{1,2}(?:st|nd|rd|th)?\s+(?:of\s+)?(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\w*|` +
		`(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\w*\s+\d{1,2}(?:st|nd|rd|th)?|` +
		`\d{4}-\d{2}-\d{2}|` +
		`\d{1,2}/\d{1,2}(?:/\d{2,4})?)\b`)
	timeRE = regexp.MustCompile(`(?i)\b(\d{1,2}(?::\d{2})?\s*(?:am|pm)|\d{1,2}:\d{2})\b`)
)

// notAName lists words commonly captured by the doctor regex that are
// not actual names ("as a doctor in your hospital" → "in").
var notAName = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "in": {}, "on": {}, "at": {},
	"by": {}, "of": {}, "to": {}, "for": {}, "with": {}, "from": {},
	"as": {}, "and": {}, "but": {}, "or": {}, "is": {}, "are": {},
	"was": {}, "were": {}, "be": {}, "today": {}, "tomorrow": {},
	"now": {}, "soon": {}, "any": {}, "all": {}, "available": {},
	"anyone": {}, "your": {}, "my": {}, "his": {}, "her": {},
	"our": {}, "their": {}, "this": {}, "that": {}, "these": {},
	"those": {}, "appointment": {}, "appointments": {}, "please": {},
}

// ExtractSlots returns whatever the regex extractors find. Missing
// slots are simply absent from the map.
func ExtractSlots(text string) map[string]string {
	slots := map[string]string{}
	if m := doctorRE.FindStringSubmatch(text); m != nil {
		cand := strings.TrimSpace(m[1])
		if _, banned := notAName[strings.ToLower(cand)]; !banned {
			slots["doctor"] = title(cand)
		}
	}
	if m := apptRE.FindStringSubmatch(text); m != nil {
		slots["appointment_id"] = strings.ToUpper(m[1])
	}
	if m := dateRE.FindStringSubmatch(text); m != nil {
		slots["date"] = strings.ToLower(m[1])
	}
	if m := timeRE.FindStringSubmatch(text); m != nil {
		slots["time"] = strings.ToLower(strings.ReplaceAll(m[1], " ", ""))
	}
	return slots
}

// nameStopRE matches the first word that signals the *name* portion has
// ended and the caller has continued into a request — e.g.
// "my name is Harsha actually have a headache" → cut at "actually".
var nameStopRE = regexp.MustCompile(
	`(?i)\b(actually|but|and|or|so|because|however|though|i|i'?m|i\s+am|` +
		`im|am|have|has|having|need|needs|needed|want|wants|wanted|would|` +
		`could|should|do|does|did|is|was|were|here|there|from|calling|` +
		`speaking|today|tomorrow|please|to|for|with|on|at|booking|book)\b.*$`)

var nameWordRE = regexp.MustCompile(`^[A-Za-z][A-Za-z'\-]*$`)

// cleanupName trims trailing junk, drops everything from the first
// stop word, and validates the remainder is a 1-4 word alphabetic name.
func cleanupName(s string) string {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ".,!?"))
	s = nameStopRE.ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(s), " ")
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	if len(words) == 0 || len(words) > 4 {
		return ""
	}
	for _, w := range words {
		if !nameWordRE.MatchString(w) {
			return ""
		}
	}
	return title(strings.Join(words, " "))
}

// ExtractName returns the patient's name from a self-introduction, or
// "" if no plausible name is found.
func ExtractName(text string) string {
	intros := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmy\s+name\s+is\s+(.+)`),
		regexp.MustCompile(`(?i)\bthis\s+is\s+(.+)`),
		regexp.MustCompile(`(?i)\bi\s*(?:'?m|\s+am)\s+(.+)`),
		regexp.MustCompile(`(?i)\bit'?s\s+(.+)`),
		regexp.MustCompile(`(?i)\bcall\s+me\s+(.+)`),
	}
	for _, re := range intros {
		if m := re.FindStringSubmatch(text); m != nil {
			if name := cleanupName(m[1]); name != "" {
				return name
			}
		}
	}
	// Strict bare-name fallback: accept only if the whole utterance is
	// 1-4 alphabetic words. Don't trim here — it produces too many
	// false positives for inputs like "medium Harshvardhan Reddy and…".
	bare := strings.TrimSpace(strings.TrimRight(text, ".,!?"))
	words := strings.Fields(bare)
	if len(words) >= 1 && len(words) <= 4 {
		for _, w := range words {
			if !nameWordRE.MatchString(w) {
				return ""
			}
		}
		return title(strings.Join(words, " "))
	}
	return ""
}

// Detect classifies an utterance, returning the highest-scoring intent.
func Detect(text string) Match {
	if strings.TrimSpace(text) == "" {
		return Match{Intent: models.IntentUnknown}
	}

	type cand struct {
		matches  int
		priority int
		intent   models.IntentType
	}
	var best *cand

	for _, r := range rules {
		hits := 0
		for _, p := range r.patterns {
			if p.MatchString(text) {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		c := cand{matches: hits, priority: r.priority, intent: r.intent}
		// More matches wins; on ties, lower priority value wins.
		if best == nil ||
			c.matches > best.matches ||
			(c.matches == best.matches && c.priority < best.priority) {
			b := c
			best = &b
		}
	}

	slots := ExtractSlots(text)
	if best == nil {
		return Match{Intent: models.IntentUnknown, Slots: slots}
	}
	conf := 0.5 + 0.2*float64(best.matches)
	if conf > 1.0 {
		conf = 1.0
	}
	return Match{Intent: best.intent, Confidence: conf, Slots: slots}
}

// title is a minimal Title-Case implementation that doesn't depend on
// the deprecated strings.Title.
func title(s string) string {
	out := make([]byte, 0, len(s))
	upcomingUpper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '-' || c == '\'':
			out = append(out, c)
			upcomingUpper = true
		case upcomingUpper:
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			out = append(out, c)
			upcomingUpper = false
		default:
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			out = append(out, c)
		}
	}
	return string(out)
}

func wordCount(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return len(strings.Fields(s))
}
