package wshandler

import (
	"math/rand"
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
