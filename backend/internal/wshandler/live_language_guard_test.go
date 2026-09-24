package wshandler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLiveLanguageGuardSwitchesOnNativeScript(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	lang, changed := guard.ObserveCustomer("ಹೌದು ಮಾಡಿದೆ")
	assert.Equal(t, "kn", lang)
	assert.True(t, changed)

	lang, changed = guard.ObserveCustomer("ప్రస్తుతం ఏమీ చేయట్లేదు")
	assert.Equal(t, "te", lang)
	assert.True(t, changed)
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

func TestLiveLanguageGuardCanReturnToClearEnglish(t *testing.T) {
	guard := newLiveLanguageGuard("kn")
	lang, changed := guard.ObserveCustomer("Now I am doing everything manually")
	assert.Equal(t, "en", lang)
	assert.True(t, changed)
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
	assert.Equal(t, liveLanguageReject, guard.ValidateAgent("Podemos realizar la demostración hoy", true))
}

func TestLiveLanguageGuardInterimDetectionDoesNotSwitchConfirmedLanguage(t *testing.T) {
	guard := newLiveLanguageGuard("en")
	language, detected := guard.ProvisionalCustomer("ప్రస్తుతం ఏమీ చేయట్లేదు")
	assert.Equal(t, "te", language)
	assert.True(t, detected)
	assert.Equal(t, "en", guard.Confirmed())

	language, changed := guard.ObserveCustomer("ప్రస్తుతం ఏమీ చేయట్లేదు")
	assert.Equal(t, "te", language)
	assert.True(t, changed)
}
