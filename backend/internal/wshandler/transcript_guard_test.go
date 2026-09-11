package wshandler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/globussoft/callified-backend/internal/llm"
)

func TestPathologicalTranscriptRejectsRepeatedASRToken(t *testing.T) {
	assert.True(t, isPathologicalTranscript(strings.Repeat("change ", 220)))
	assert.False(t, isPathologicalTranscript("No, I didn't get it. Can you repeat please?"))
	assert.False(t, isPathologicalTranscript("We are doing all lead qualification and follow-ups manually today."))
}

func TestRepeatRequestsAreTerminallyProtectedAcrossLanguages(t *testing.T) {
	for _, text := range []string{
		"No, I didn't get it. Can you repeat please?",
		"మళ్లీ చెప్పండి, నాకు అర్థం కాలేదు",
		"फिर से बताइए, समझ नहीं आया",
		"மீண்டும் சொல்லுங்கள், புரியவில்லை",
	} {
		assert.True(t, isRepeatOrClarificationRequest(text), text)
		assert.False(t, terminalActionAllowed("customer_requested_end", text, nil), text)
	}
}

func TestBargeInPartialNeedsMeaningfulSpeech(t *testing.T) {
	assert.False(t, isMeaningfulBargeInPartial("ch"))
	assert.False(t, isMeaningfulBargeInPartial("change change change"))
	assert.False(t, isMeaningfulBargeInPartial(strings.Repeat("change ", 30)))
	assert.True(t, isMeaningfulBargeInPartial("no"))
	assert.True(t, isMeaningfulBargeInPartial("can you repeat"))
}

func TestAppointmentTerminalActionRequiresDayAndExactTime(t *testing.T) {
	history := []llm.ChatMessage{{Role: "model", Text: "What day and time works for you?"}}
	assert.False(t, terminalActionAllowed("appointment_booked", "Tomorrow", history))
	assert.True(t, terminalActionAllowed("appointment_booked", "Tomorrow at three PM", history))

	history = []llm.ChatMessage{
		{Role: "user", Text: "Today"},
		{Role: "model", Text: "What exact time works today?"},
	}
	assert.True(t, terminalActionAllowed("appointment_booked", "12 o'clock", history))
}

func TestAppointmentTerminalActionAcceptsCalendarDateFromPriorTurn(t *testing.T) {
	tests := []struct {
		name    string
		dateAsk string
	}{
		{name: "word ordinal", dateAsk: "On the twentieth of this month, what time works best?"},
		{name: "numeric ordinal", dateAsk: "Does the 20th work for your demo?"},
		{name: "day then month", dateAsk: "What time works on 20 September?"},
		{name: "month then day", dateAsk: "What time works on September 20?"},
		{name: "numeric date", dateAsk: "What time works on 20/09?"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			history := []llm.ChatMessage{{Role: "model", Text: test.dateAsk}}
			assert.True(t, terminalActionAllowed("appointment_booked", "Okay for me like 12 PM", history))
		})
	}
}

func TestAppointmentTerminalActionDoesNotTreatTimeAsDate(t *testing.T) {
	history := []llm.ChatMessage{{Role: "model", Text: "What exact time works for the demo?"}}
	assert.False(t, terminalActionAllowed("appointment_booked", "12 PM", history))
}

func TestAppointmentTerminalActionSupportsEveryCallLanguage(t *testing.T) {
	tests := []struct {
		language string
		answer   string
	}{
		{language: "English", answer: "Wednesday at twelve PM"},
		{language: "Hindi", answer: "बुधवार दोपहर बारह बजे"},
		{language: "Marathi", answer: "बुधवारी दुपारी बारा वाजता"},
		{language: "Telugu", answer: "బుధవారం మధ్యాహ్నం పన్నెండు గంటలకి"},
		{language: "Telugu STT transliteration", answer: "వెడ్నెస్డే ట్వెల్వ్ పీఎం"},
		{language: "Tamil", answer: "புதன்கிழமை மதியம் பன்னிரண்டு மணி"},
		{language: "Kannada", answer: "ಬುಧವಾರ ಮಧ್ಯಾಹ್ನ ಹನ್ನೆರಡು ಗಂಟೆಗೆ"},
		{language: "Malayalam", answer: "ബുധനാഴ്ച ഉച്ചയ്ക്ക് പന്ത്രണ്ട് മണിക്ക്"},
		{language: "Bengali", answer: "বুধবার দুপুর বারোটা"},
		{language: "Gujarati", answer: "બુધવાર બપોરે બાર વાગ્યે"},
		{language: "Punjabi", answer: "ਬੁੱਧਵਾਰ ਦੁਪਹਿਰ ਬਾਰਾਂ ਵਜੇ"},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			assert.True(t, terminalActionAllowed("appointment_booked", test.answer, nil), test.answer)
		})
	}
}

func TestAppointmentDateSurvivesClarificationTurns(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "user", Text: "వెడ్నెస్డే ట్వెల్వ్ పీఎం"},
		{Role: "model", Text: "Please give the exact time."},
		{Role: "user", Text: "12 పీఎం"},
		{Role: "model", Text: "Please give the exact time."},
		{Role: "user", Text: "నేను చెప్పాను కదా"},
		{Role: "model", Text: "Is that noon or midnight?"},
	}
	assert.True(t, terminalActionAllowed("appointment_booked", "మధ్యాహ్నం పన్నెండు గంటలకి", history))
}

func TestTerminalActionRequiresExplicitDeclineOrEnd(t *testing.T) {
	assert.False(t, terminalActionAllowed("customer_declined", "Can you explain it?", nil))
	assert.True(t, terminalActionAllowed("customer_declined", "No thanks, I am not interested", nil))
	assert.True(t, terminalActionAllowed("customer_declined", "No", nil))
	assert.False(t, terminalActionAllowed("customer_requested_end", "Please repeat that", nil))
	assert.True(t, terminalActionAllowed("customer_requested_end", "Please end the call", nil))
}
