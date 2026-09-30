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

// liveLanguageGuard keeps Gemini Live locked to one reply language. Customer
// speech, script, accent, and model language guesses are never switch
// authority; only an explicit request in the final customer transcript is.
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

// ApplyExplicitSwitch accepts a semantic switch_language tool call after
// Gemini has understood an explicit customer request. The supported-language
// allowlist remains server-owned so arbitrary model output cannot alter state.
func (g *liveLanguageGuard) ApplyExplicitSwitch(target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if _, supported := langLabels[target]; !supported {
		return false
	}
	g.mu.Lock()
	g.confirmed = target
	g.expected = target
	g.mu.Unlock()
	return true
}

// Expected returns the currently locked reply language.
func (g *liveLanguageGuard) Expected() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.expected
}

func (g *liveLanguageGuard) ObserveCustomer(text string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	target, requested := isExplicitLangSwitch(text)
	if !requested {
		g.expected = g.confirmed
		return g.confirmed, false
	}
	g.expected = target
	if target == g.confirmed {
		return g.confirmed, false
	}
	g.confirmed = target
	return target, true
}

// ProvisionalCustomer may recognize an explicit request early, but it never
// mutates state. The final transcript remains the only switch authority.
func (g *liveLanguageGuard) ProvisionalCustomer(text string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	target, requested := isExplicitLangSwitch(text)
	return target, requested && target != g.confirmed
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
