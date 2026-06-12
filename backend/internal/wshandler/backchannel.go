package wshandler

import (
	"strings"
)

// fillerSounds is a set of short background-noise transcripts that Sarvam
// commonly returns for non-speech sounds (breathing, ambient noise, filler
// syllables). These are dropped before reaching the pipeline so the agent
// keeps waiting for a real customer reply.
var fillerSounds = map[string]struct{}{
	"hu": {}, "ha": {}, "haa": {}, "hah": {},
	"hm": {}, "hmm": {}, "hmmm": {},
	"ah": {}, "ahh": {}, "aah": {},
	"oh": {}, "ohh": {},
	"uh": {}, "uhh": {},
	"um": {}, "umm": {}, "ummm": {},
	"ugh": {},
	"eh": {}, "ehh": {},
	"ow": {},
	// Indian-language equivalents
	"haan": {}, "han": {},
	"ho": {},
	"hn": {},
}

// isFillerSound returns true when the entire transcript is a single short
// background-noise syllable that should not be forwarded to the pipeline.
func isFillerSound(text string) bool {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(words) != 1 {
		return false // multi-word transcripts are always real speech
	}
	word := strings.Trim(words[0], ".,!?…")
	if _, ok := fillerSounds[word]; ok {
		return true
	}
	// Also drop any single word that is 1–2 characters — too short to be intent.
	return len([]rune(word)) <= 2
}


// explicitSwitchKeywords maps target language codes to phrases that unambiguously
// request a language switch, regardless of what Sarvam detects as the language.
// e.g. "can you speak in kannada" → Sarvam returns "en" but we force-switch to "kn".
var explicitSwitchKeywords = map[string][]string{
	"en": {
		"switch to english", "speak in english", "speak english",
		"english please", "can you speak in english", "english lo",
		"english mein", "in english", "english mein baat",
	},
	"hi": {
		"switch to hindi", "speak in hindi", "speak hindi",
		"hindi please", "can you speak in hindi", "hindi mein baat karo",
		"hindi lo", "in hindi", "hindi mein",
	},
	"te": {
		"switch to telugu", "speak in telugu", "speak telugu",
		"telugu please", "can you speak in telugu", "telugu lo matladandi",
		"telugu lo", "in telugu",
	},
	"ta": {
		"switch to tamil", "speak in tamil", "speak tamil",
		"tamil please", "can you speak in tamil", "tamil la pesunga",
		"tamil la", "in tamil",
	},
	"kn": {
		"switch to kannada", "speak in kannada", "speak kannada",
		"kannada please", "can you speak in kannada", "kannada alli",
		"kannada lo", "in kannada",
	},
	"ml": {
		"switch to malayalam", "speak in malayalam", "speak malayalam",
		"malayalam please", "can you speak in malayalam", "in malayalam",
	},
	"mr": {
		"switch to marathi", "speak in marathi", "speak marathi",
		"marathi please", "can you speak in marathi", "marathi madhe",
		"in marathi",
	},
	"gu": {
		"switch to gujarati", "speak in gujarati", "speak gujarati",
		"gujarati please", "can you speak in gujarati", "in gujarati",
	},
	"pa": {
		"switch to punjabi", "speak in punjabi", "speak punjabi",
		"punjabi please", "can you speak in punjabi", "in punjabi",
	},
	"bn": {
		"switch to bengali", "speak in bengali", "speak bengali",
		"bengali please", "can you speak in bengali", "in bengali",
	},
}

// scriptBlock defines the primary Unicode range for an Indian language script.
type scriptBlock struct{ lo, hi rune }

var langScriptBlocks = map[string]scriptBlock{
	"hi": {0x0900, 0x097F}, // Devanagari — Hindi
	"mr": {0x0900, 0x097F}, // Devanagari — Marathi
	"ta": {0x0B80, 0x0BFF}, // Tamil
	"te": {0x0C00, 0x0C7F}, // Telugu
	"kn": {0x0C80, 0x0CFF}, // Kannada
	"ml": {0x0D00, 0x0D7F}, // Malayalam
	"bn": {0x0980, 0x09FF}, // Bengali
	"gu": {0x0A80, 0x0AFF}, // Gujarati
	"pa": {0x0A00, 0x0A7F}, // Gurmukhi — Punjabi
}

// scriptMatchesLang returns true when the non-ASCII characters in text are
// consistent with the given language code. This catches Sarvam mis-detections
// where the customer speaks Kannada but gets labelled ta-IN, or speaks Hindi
// but gets labelled pa-IN — because Kannada characters cannot appear in Tamil
// Unicode and vice-versa.
//
// Returns true for English ("en") and unknown languages (no script to check).
// Returns false when the transcript has no non-ASCII characters — a pure-ASCII
// transcript (English digits/words) should not trigger a language switch.
func scriptMatchesLang(text, lang string) bool {
	b, ok := langScriptBlocks[lang]
	if !ok {
		return true // en or unknown — no script check
	}
	total, matched := 0, 0
	for _, r := range text {
		if r > 127 {
			total++
			if r >= b.lo && r <= b.hi {
				matched++
			}
		}
	}
	if total == 0 {
		return false // pure ASCII — not enough signal to confirm a script switch
	}
	return matched*10 >= total*6 // ≥60% of non-ASCII chars must match
}

// isExplicitLangSwitch checks if the transcript contains a clear language-switch
// request. Returns the target language code and true if found.
func isExplicitLangSwitch(text string) (targetLang string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	for lang, keywords := range explicitSwitchKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return lang, true
			}
		}
	}
	return "", false
}

