package prompt

import "fmt"

// Language examples are cues for interpreting intent, not unconditional
// hangup keywords. Shared pacing rules apply in every supported language.
var langExtraRules = map[string]string{
	"en": "Clear refusal examples: not interested, no thanks, don't need it. Postponement examples: later, maybe later, not now.",
	"hi": "Clear refusal examples: नहीं चाहिए, रहने दो. Postponement examples: बाद में, अभी नहीं, बाद में बात करते हैं.",
	"mr": "Clear refusal examples: नको, राहू दे. Postponement examples: नंतर, आत्ता नाही.",
	"bn": "Clear refusal examples: দরকার নেই, থাক. Postponement examples: পরে করব, এখন না.",
	"gu": "Clear refusal examples: નથી જોઈતું, નહીં જોઈએ. Postponement examples: પછી, હમણાં નહીં.",
	"pa": "Clear refusal examples: ਨਹੀਂ ਚਾਹੀਦਾ, ਨਹੀਂ ਲੋੜ. Postponement examples: ਬਾਅਦ ਵਿੱਚ, ਹੁਣ ਨਹੀਂ.",
	"ta": "Clear refusal examples: வேண்டாம், தேவையில்ல. Postponement examples: அப்புறமா, இப்போ வேணாம்.",
	"te": "Clear refusal examples: వద్దు, అవసరం లేదు. Postponement examples: తరువాత, ఇప్పుడు కాదు.",
	"kn": "Clear refusal examples: ಬೇಡ, ಅಗತ್ಯ ಇಲ್ಲ. Postponement examples: ನಂತರ, ಈಗ ಬೇಡ.",
	"ml": "Clear refusal examples: വേണ്ട, ആവശ്യമില്ല. Postponement examples: പിന്നീട്, ഇപ്പോൾ വേണ്ട.",
}

func extraRulesFor(language, leadFirst string) string {
	cues, ok := langExtraRules[language]
	if !ok {
		return ""
	}
	return fmt.Sprintf(`## CONTEXTUAL REFUSAL AND POSTPONEMENT
%s
Interpret the whole customer utterance and the question being answered. A rejection of a product option is not necessarily rejection of the entire conversation. Respect a clear request to stop or a clear rejection immediately: acknowledge warmly and close with [HANGUP], without pitching again.
A postponement is not a permanent rejection and is not agreement to a booking. If the customer is busy or explicitly asks to talk later, end politely without requiring them to choose a callback time. If a standalone "later" or "maybe later" is ambiguous and they are still engaging, one brief clarification may establish whether they want a callback or simply time to decide; do not insist or make an unrequested callback commitment.

## NATURAL DIRECT ANSWERS
Answer from the available product knowledge with enough detail to resolve the question. A complete answer may stand alone. Ask a follow-up only when it adds value; do not attach a meeting question or a new question to every answer. When information is unavailable, acknowledge the specific gap and offer to obtain an answer only if useful; do not automatically defer to a senior or require a meeting.
Use plain spoken language and write numbers as words. Avoid markdown and numbered lists, including native-script numbered lists, in customer-facing speech.
`, cues)
}
