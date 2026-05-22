package wshandler

import (
	"math/rand"
	"regexp"
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

// fillersByLang maps language codes to conversational filler words.
// These are injected as the first TTS item when the user speaks >2 words,
// giving a more natural response cadence while the LLM generates its reply.
var fillersByLang = map[string][]string{
	"hi": {"Hmm...", "Achha...", "Okay...", "Haan..."},
	"mr": {"Hmm...", "Achha...", "Theek ahe...", "Ho..."},
	"en": {"Hmm...", "I see...", "Okay...", "Right..."},
	"ta": {"Hmm...", "Sari...", "Okay...", "Aama..."},
	"te": {"Hmm...", "Sare...", "Okay...", "Avunu..."},
	"bn": {"Hmm...", "Achha...", "Okay...", "Haan..."},
	"gu": {"Hmm...", "Saru...", "Okay...", "Ha..."},
	"kn": {"Hmm...", "Sari...", "Okay...", "Houdu..."},
	"ml": {"Hmm...", "Sari...", "Okay...", "Athe..."},
	"pa": {"Hmm...", "Achha...", "Okay...", "Haan..."},
}

// defaultFillers is used when the language has no specific mapping.
var defaultFillers = []string{"Hmm...", "Okay...", "I see..."}

// randomFiller returns a random filler phrase for the given language code.
// Mirrors Python ws_handler.py:
//
//	fillers = ["Hmm...", "Achha...", "Okay...", "Haan..."]
//	await tts_queue.put(random.choice(fillers))
func randomFiller(language string) string {
	fillers, ok := fillersByLang[language]
	if !ok {
		fillers = defaultFillers
	}
	return fillers[rand.Intn(len(fillers))]
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

// englishNumberWords covers digits spoken aloud; used to detect phone-number
// utterances that shouldn't trigger a language switch.
var englishNumberWords = map[string]struct{}{
	"zero": {}, "one": {}, "two": {}, "three": {}, "four": {}, "five": {},
	"six": {}, "seven": {}, "eight": {}, "nine": {}, "ten": {},
	"eleven": {}, "twelve": {}, "thirteen": {}, "fourteen": {}, "fifteen": {},
	"sixteen": {}, "seventeen": {}, "eighteen": {}, "nineteen": {}, "twenty": {},
	"thirty": {}, "forty": {}, "fifty": {}, "sixty": {}, "seventy": {}, "eighty": {}, "ninety": {},
	"hundred": {}, "thousand": {}, "lakh": {}, "crore": {}, "number": {},
}

var digitRunRe = regexp.MustCompile(`\d{4,}`)

// isNumberInput returns true when the transcript is primarily digits or
// English number words — these are language-neutral and must not be used
// to trigger a language switch (e.g. customer gives phone number).
func isNumberInput(text string) bool {
	if digitRunRe.MatchString(text) {
		return true
	}
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return false
	}
	count := 0
	for _, w := range words {
		w = strings.Trim(w, ".,!?")
		if _, ok := englishNumberWords[w]; ok {
			count++
		}
	}
	return count*2 > len(words)
}
