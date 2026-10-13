package wshandler

import (
	"strings"
)

// fillerSounds is a set of short background-noise transcripts that Sarvam
// commonly returns for non-speech sounds (breathing, ambient noise, filler
// syllables). These are dropped before reaching the pipeline so the agent
// keeps waiting for a real customer reply.
var fillerSounds = map[string]struct{}{
	"hu": {}, "ha": {}, "haa": {}, "hah": {}, "haha": {},
	"hm": {}, "hmm": {}, "hmmm": {}, "hmmmm": {},
	"mm": {}, "mmm": {}, "mmmm": {},
	"mhm": {}, "mmhmm": {}, "mhmhm": {},
	"ah": {}, "ahh": {}, "aah": {}, "aahh": {},
	"oh": {}, "ohh": {}, "ooh": {},
	"uh": {}, "uhh": {}, "uhm": {},
	"um": {}, "umm": {}, "ummm": {},
	"er": {}, "err": {},
	"ugh": {},
	"eh":  {}, "ehh": {},
	"ow": {},
	// Indian-language equivalents
	"hn": {}, "hunn": {},
	"హమ్": {}, "హ్మ్": {}, "ఉమ్": {}, "అమ్": {},
	"ह्म्म": {}, "हम्म": {}, "उम्": {}, "अम्": {},
}

// isFillerSound returns true when the entire transcript is a single short
// background-noise syllable that should not be forwarded to the pipeline.
func isFillerSound(text string) bool {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(words) != 1 {
		return false // multi-word transcripts are always real speech
	}
	word := normalizeFillerWord(words[0])
	if word == "" {
		return strings.TrimSpace(text) != ""
	}
	if _, ok := fillerSounds[word]; ok {
		return true
	}
	return false
}

// isKnownFiller returns true only when the text exactly matches a known filler
// sound. Unlike isFillerSound, it does not drop short partial words like "he"
// or "my", which are often the beginning of a real interruption.
func isKnownFiller(text string) bool {
	word := normalizeFillerWord(text)
	if word == "" {
		return strings.TrimSpace(text) != ""
	}
	_, ok := fillerSounds[word]
	return ok
}

func normalizeFillerWord(text string) string {
	word := strings.ToLower(strings.TrimSpace(text))
	word = strings.Trim(word, ".,!?…:;\"'`()[]{}")
	replacer := strings.NewReplacer("-", "", "_", "", " ", "", "'", "", "’", "")
	return replacer.Replace(word)
}

// explicitSwitchKeywords maps target language codes to phrases that unambiguously
// request a language switch, regardless of what Sarvam detects as the language.
// Includes:
//   - English phrases (e.g. "speak in Kannada")
//   - Romanized/transliterated requests (e.g. "kannada alli mathadi")
//   - Native-script phrases (e.g. "ಕನ್ನಡದಲ್ಲಿ", "हिंदी में") so the request works
//     when the customer asks in their own language.
var explicitSwitchKeywords = map[string][]string{
	"en": {
		"switch to english", "speak in english", "speak english", "talk in english", "talk english",
		"english please", "can you speak in english", "can you talk in english", "english mein baat karo",
		"ഇംഗ്ലീഷിൽ പറയു", "इंग्लिश में बोलो", "ஆங்கிலத்தில் பேசுங்கள்", "ఆంగ్లంలో మాట్లాడండి", "ಇಂಗ್ಲಿಷ್ನಲ್ಲಿ ಮಾತಾಡಿ",
	},
	"hi": {
		"switch to hindi", "speak in hindi", "speak hindi", "talk in hindi", "talk hindi",
		"hindi please", "can you speak in hindi", "can you talk in hindi", "hindi mein baat karo",
		"hindi mein bolo",
		"हिंदी में बात", "हिंदी में बोलो",
		"हिंदी बोलो", "hindi bol",
	},
	"te": {
		"switch to telugu", "speak in telugu", "speak telugu", "talk in telugu", "talk telugu",
		"telugu please", "can you speak in telugu", "can you talk in telugu", "telugu lo matladandi",
		"telugu matladu", "telugu lo matladi",
		"తెలుగులో మాట్లాడండి",
	},
	"ta": {
		"switch to tamil", "speak in tamil", "speak tamil", "talk in tamil", "talk tamil",
		"tamil please", "can you speak in tamil", "can you talk in tamil", "tamil la pesunga",
		"tamil pesu", "tamil la pesu",
		"தமிழில் பேசுங்கள்",
	},
	"kn": {
		"switch to kannada", "speak in kannada", "speak kannada", "talk in kannada", "talk kannada",
		"kannada please", "can you speak in kannada", "can you talk in kannada",
		"kannada dhalli mathadi",
		"kannada mathadi", "kannada alli mathadi", "kannada bari",
		"ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡಿ",
		"ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡ್ತೀರಾ", "kannada alli matadi",
	},
	"ml": {
		"switch to malayalam", "speak in malayalam", "speak malayalam", "talk in malayalam", "talk malayalam",
		"malayalam please", "can you speak in malayalam", "can you talk in malayalam",
		"malayalam parayu", "malayalam il parayu", "malayalam parayanam",
		"മലയാളത്തിൽ പറയു",
	},
	"mr": {
		"switch to marathi", "speak in marathi", "speak marathi", "talk in marathi", "talk marathi",
		"marathi please", "can you speak in marathi", "can you talk in marathi",
		"marathi bol", "marathi madhe bol",
		"मराठीत बोला", "मराठी मध्ये बोल",
	},
	"gu": {
		"switch to gujarati", "speak in gujarati", "speak gujarati", "talk in gujarati", "talk gujarati",
		"gujarati please", "can you speak in gujarati", "can you talk in gujarati",
		"gujarati ma bolo", "gujarati bol",
		"ગુજરાતીમાં બોલો",
	},
	"pa": {
		"switch to punjabi", "speak in punjabi", "speak punjabi", "talk in punjabi", "talk punjabi",
		"punjabi please", "can you speak in punjabi", "can you talk in punjabi",
		"punjabi vich bolo", "panjabi me bolo",
		"panjabi mein bolo", "punjabi mein bolo", "punjabi bol",
		"ਪੰਜਾਬੀ ਵਿੱਚ ਬੋਲੋ",
	},
	"bn": {
		"switch to bengali", "speak in bengali", "speak bengali", "talk in bengali", "talk bengali",
		"bengali please", "can you speak in bengali", "can you talk in bengali",
		"bengali te bolo", "bengali bolo", "bangla te bolo",
		"বাংলায় কথা", "বাংলায় বলো",
	},
}

// isExplicitLangSwitch checks if the transcript contains a clear language-switch
// request. Returns the target language code and true if found.
func isExplicitLangSwitch(text string) (targetLang string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	bestLang, bestIndex := "", len(lower)+1
	for lang, keywords := range explicitSwitchKeywords {
		for _, kw := range keywords {
			for searchFrom := 0; searchFrom < len(lower); {
				relativeIndex := strings.Index(lower[searchFrom:], kw)
				if relativeIndex < 0 {
					break
				}
				index := searchFrom + relativeIndex
				if !switchRequestIsNegated(lower, index) && index < bestIndex {
					bestLang, bestIndex = lang, index
				}
				searchFrom = index + len(kw)
			}
		}
	}
	return bestLang, bestLang != ""
}

// switchRequestIsNegated limits negation to the clause containing the matched
// request. This lets "don't speak Hindi, speak English" select English while
// still rejecting its first, negated request.
func switchRequestIsNegated(text string, matchIndex int) bool {
	clauseStart := 0
	for _, separator := range []string{",", ";", ".", "!", "?", " but ", " however "} {
		if index := strings.LastIndex(text[:matchIndex], separator); index >= clauseStart {
			clauseStart = index + len(separator)
		}
	}
	prefix := strings.TrimSpace(text[clauseStart:matchIndex])
	for _, negation := range []string{
		"don't", "dont", "do not", "not", "no need to", "stop", "can't", "cant", "cannot",
		"mat", "नहीं", "ಬೇಡ", "వద్దు",
	} {
		if strings.Contains(prefix, negation) {
			return true
		}
	}
	return false
}
