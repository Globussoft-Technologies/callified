package wshandler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLiveLanguageGuardDoesNotSwitchOnNativeScript(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	lang, changed := guard.ObserveCustomer("ಹೌದು ಮಾಡಿದೆ")
	assert.Equal(t, "en", lang)
	assert.False(t, changed)
	assert.Equal(t, "en", guard.Expected())

	lang, changed = guard.ObserveCustomer("ప్రస్తుతం ఏమీ చేయట్లేదు")
	assert.Equal(t, "en", lang)
	assert.False(t, changed)
}

func TestLiveLanguageGuardKeepsLanguageForAmbiguousShortLatinText(t *testing.T) {
	guard := newLiveLanguageGuard("kn")
	lang, changed := guard.ObserveCustomer("decent")
	assert.Equal(t, "kn", lang)
	assert.False(t, changed)

	lang, changed = guard.ObserveCustomer("But number")
	assert.Equal(t, "kn", lang)
	assert.False(t, changed)
}

func TestLiveLanguageGuardDoesNotSwitchBecauseCustomerUsesEnglish(t *testing.T) {
	guard := newLiveLanguageGuard("kn")
	lang, changed := guard.ObserveCustomer("Now I am doing everything manually")
	assert.Equal(t, "kn", lang)
	assert.False(t, changed)
	assert.Equal(t, "kn", guard.Expected())
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("I understand. Let me explain the next step.", true))
}

func TestLiveLanguageGuardRejectsWrongOrMixedAgentLanguage(t *testing.T) {
	guard := newLiveLanguageGuard("te")
	assert.Equal(t, liveLanguageAccept, guard.ValidateAgent("ఇది మీ సేల్స్ ప్రక్రియను సులభం చేస్తుంది", false))
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("ಇದು ನಿಮ್ಮ ಸೇಲ್ಸ್ ಪ್ರಕ್ರಿಯೆಯನ್ನು ಸುಲಭಗೊಳಿಸುತ್ತದೆ", false))
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("ఇది చాలా ಒಳ್ಳೆಯದು ಮತ್ತು ನಿಮ್ಮ ಮಾರಾಟಕ್ಕೆ ಸಹಾಯ ಮಾಡುತ್ತದೆ", false))
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("Podemos realizar la demostración hoy", true))
}

func TestLiveLanguageGuardAllowsEnglishProductTermsBeforeNativeScript(t *testing.T) {
	guard := newLiveLanguageGuard("kn")
	assert.Equal(t, liveLanguagePending, guard.ValidateAgent("GlobusCRM AI", false))
	assert.Equal(t, liveLanguageAccept, guard.ValidateAgent("GlobusCRM AI ನಿಮ್ಮ ಮಾರಾಟ ಪ್ರಕ್ರಿಯೆಯನ್ನು ಸುಲಭಗೊಳಿಸುತ್ತದೆ", false))
}

func TestLiveLanguageGuardDoesNotTreatAnyLatinLanguageAsEnglish(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	assert.Equal(t, liveLanguageAccept, guard.ValidateAgent("Hello, how can I help you today?", false))
	guard.ObserveCustomer("Podemos continuar con la demostración")
	assert.Equal(t, "en", guard.Expected())
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("Podemos realizar la demostración hoy", true))
}

func TestLiveLanguageGuardDoesNotSwitchOnRomanizedLanguage(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	language, changed := guard.ObserveCustomer("Aah naaku ee roju madhyanam 12:00 ki")
	assert.Equal(t, "en", language)
	assert.False(t, changed)
	assert.Equal(t, "en", guard.Expected())
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("సరే, మధ్యాహ్నం పన్నెండు గంటలకు కలుద్దాం.", false))
}

func TestLiveLanguageGuardDoesNotForceEnglishFromCorruptedLatinTranscript(t *testing.T) {
	guard := newLiveLanguageGuard("kn")
	language, changed := guard.ObserveCustomer("Aapko Heroite Saripota the Hero Kudurthada?")
	assert.Equal(t, "kn", language)
	assert.False(t, changed)
	assert.Equal(t, "kn", guard.Expected())
	assert.Equal(t, liveLanguageAccept, guard.ValidateAgent("ಹೌದು, ವಿವರಿಸುತ್ತೇನೆ.", false))
}

func TestLiveLanguageGuardSwitchesOnlyOnExplicitRequest(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	language, detected := guard.ProvisionalCustomer("ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡಿ")
	assert.Equal(t, "kn", language)
	assert.True(t, detected)
	assert.Equal(t, "en", guard.Confirmed())

	language, changed := guard.ObserveCustomer("ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡಿ")
	assert.Equal(t, "kn", language)
	assert.True(t, changed)
	assert.Equal(t, "kn", guard.Expected())
}

func TestLiveLanguageGuardIgnoresNonRequestInterimAndNegatedRequest(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	language, detected := guard.ProvisionalCustomer("ప్రస్తుతం ఏమీ చేయట్లేదు")
	assert.Empty(t, language)
	assert.False(t, detected)

	language, changed := guard.ObserveCustomer("Please do not speak in Hindi")
	assert.Equal(t, "en", language)
	assert.False(t, changed)
}

func TestLiveLanguageGuardDoesNotSwitchOnLanguageMention(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	for _, transcript := range []string{
		"I studied in Hindi medium",
		"The document is available in Marathi",
		"Our customer support course is in Telugu",
	} {
		language, changed := guard.ObserveCustomer(transcript)
		assert.Equal(t, "en", language)
		assert.False(t, changed, transcript)
	}
}

func TestLiveLanguageGuardScopesNegationToMatchedRequest(t *testing.T) {
	guard := newLiveLanguageGuard("te")
	language, changed := guard.ObserveCustomer("Don't speak in Hindi, speak in English")
	assert.Equal(t, "en", language)
	assert.True(t, changed)
}

func TestLiveLanguageGuardAcceptsSemanticToolSwitchForNaturalVariation(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	assert.True(t, guard.ApplyExplicitSwitch("te"))
	assert.Equal(t, "te", guard.Confirmed())
	assert.Equal(t, "te", guard.Expected())
	assert.False(t, guard.ApplyExplicitSwitch("unsupported"))
	assert.Equal(t, "te", guard.Confirmed())
}
