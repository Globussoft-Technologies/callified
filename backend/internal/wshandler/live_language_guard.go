package wshandler

import (
	"strings"
	"sync"
	"unicode"
)

type liveLanguageVerdict uint8

const (
	liveLanguagePending liveLanguageVerdict = iota
	liveLanguageAccept
	liveLanguageReject
)

// liveLanguageGuard keeps language selection deterministic around Gemini Live.
// Native-script evidence is reliable enough to switch immediately; ambiguous
// short Latin transcripts retain the previously confirmed language.
type liveLanguageGuard struct {
	mu        sync.RWMutex
	confirmed string
	expected  string
}

func newLiveLanguageGuard(initial string) *liveLanguageGuard {
	if _, ok := langLabels[initial]; !ok {
		initial = "en"
	}
	return &liveLanguageGuard{confirmed: initial, expected: initial}
}

func (g *liveLanguageGuard) Confirmed() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.confirmed
}

// Expected returns the language Callified may safely enforce for the current
// customer turn. An empty value means the transcript is ambiguous (commonly a
// Romanized Indian language), so Gemini Live must follow the audio naturally.
func (g *liveLanguageGuard) Expected() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.expected
}

func (g *liveLanguageGuard) ObserveCustomer(text string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	detected := detectCustomerLanguage(text, g.confirmed)
	if detected == "" {
		g.expected = ""
		return g.confirmed, false
	}
	g.expected = detected
	if detected == g.confirmed {
		return g.confirmed, false
	}
	g.confirmed = detected
	return detected, true
}

// ProvisionalCustomer inspects Gemini's low-latency interim transcript without
// mutating the confirmed language. Final input transcription remains the only
// authority that can switch a call, so unstable interim hypotheses cannot
// cause false language changes.
func (g *liveLanguageGuard) ProvisionalCustomer(text string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	detected := detectCustomerLanguage(text, g.confirmed)
	return detected, detected != "" && detected != g.confirmed
}

func detectCustomerLanguage(text, current string) string {
	if explicit, ok := isExplicitLangSwitch(text); ok {
		return explicit
	}
	detected := dominantSupportedScript(text, current)
	return detected
}

// ValidateAgent returns pending until enough output text exists to make an
// early decision. final=true always resolves a non-empty response.
func (g *liveLanguageGuard) ValidateAgent(text string, final bool) liveLanguageVerdict {
	g.mu.RLock()
	defer g.mu.RUnlock()
	text = strings.TrimSpace(text)
	if text == "" {
		if final {
			return liveLanguageReject
		}
		return liveLanguagePending
	}

	counts, latin := supportedScriptCounts(text)
	expected := g.expected
	if expected == "" {
		// Gemini Live receives the original audio and can infer a Romanized or
		// mixed language more reliably than Callified can from Latin text. Once
		// output transcription begins, release it without imposing a language.
		if latin >= 4 {
			return liveLanguageAccept
		}
		for _, count := range counts {
			if count >= 2 {
				return liveLanguageAccept
			}
		}
		if final {
			return liveLanguageAccept
		}
		return liveLanguagePending
	}
	if expected == "en" {
		for _, count := range counts {
			if count >= 3 {
				return liveLanguageReject
			}
		}
		if latin >= 4 && hasEnglishEvidence(text) {
			return liveLanguageAccept
		}
		if final {
			return liveLanguageReject
		}
		return liveLanguagePending
	}

	expectedCount := counts[expected]
	for language, count := range counts {
		if language != expected && count >= 3 {
			return liveLanguageReject
		}
	}
	// Hold roughly the first phrase so a response that begins correctly but
	// then mixes another script is rejected before its audio is played.
	if expectedCount >= 12 {
		return liveLanguageAccept
	}
	if final {
		if expectedCount > 0 {
			return liveLanguageAccept
		}
		return liveLanguageReject
	}
	return liveLanguagePending
}

func dominantSupportedScript(text, current string) string {
	counts, _ := supportedScriptCounts(text)
	bestLanguage, bestCount, total := "", 0, 0
	for language, count := range counts {
		total += count
		if count > bestCount {
			bestLanguage, bestCount = language, count
		}
	}
	if bestCount < 2 || bestCount*100 < total*70 {
		return ""
	}
	// Hindi and Marathi share Devanagari. Preserve either when already known;
	// otherwise default to Hindi until the customer explicitly requests Marathi.
	if bestLanguage == "hi" && current == "mr" {
		return "mr"
	}
	return bestLanguage
}

func supportedScriptCounts(text string) (map[string]int, int) {
	counts := map[string]int{"hi": 0, "ta": 0, "te": 0, "kn": 0, "bn": 0, "gu": 0, "pa": 0, "ml": 0}
	latin := 0
	for _, r := range text {
		switch {
		case r >= 0x0900 && r <= 0x097f:
			counts["hi"]++
		case r >= 0x0980 && r <= 0x09ff:
			counts["bn"]++
		case r >= 0x0a00 && r <= 0x0a7f:
			counts["pa"]++
		case r >= 0x0a80 && r <= 0x0aff:
			counts["gu"]++
		case r >= 0x0b80 && r <= 0x0bff:
			counts["ta"]++
		case r >= 0x0c00 && r <= 0x0c7f:
			counts["te"]++
		case r >= 0x0c80 && r <= 0x0cff:
			counts["kn"]++
		case r >= 0x0d00 && r <= 0x0d7f:
			counts["ml"]++
		case unicode.Is(unicode.Latin, r) && unicode.IsLetter(r):
			latin++
		}
	}
	return counts, latin
}

func hasEnglishEvidence(text string) bool {
	words := strings.Fields(strings.ToLower(text))
	common := map[string]bool{
		"i": true, "i'm": true, "am": true, "are": true, "is": true,
		"the": true, "this": true, "that": true, "what": true, "how": true,
		"you": true, "your": true, "we": true, "my": true, "not": true,
		"hi": true, "hello": true, "yes": true, "no": true, "can": true,
		"do": true, "does": true, "have": true, "would": true, "please": true,
		"thanks": true, "thank": true, "good": true, "great": true,
	}
	for _, word := range words {
		word = strings.Trim(word, ".,?!;:\"'()")
		if common[word] {
			return true
		}
	}
	return false
}
