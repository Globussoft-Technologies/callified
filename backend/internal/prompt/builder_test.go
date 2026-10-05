package prompt

import (
	"strings"
	"testing"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/assert"
)

func TestRenderCallMemoryEmpty(t *testing.T) {
	assert.Equal(t, "", renderCallMemory(nil))
	assert.Equal(t, "", renderCallMemory([]db.CallMemory{}))
}

func TestRenderCallMemoryIncludesFields(t *testing.T) {
	out := renderCallMemory([]db.CallMemory{
		{
			CreatedAt: "2026-09-01",
			Summary:   "Customer asked about EMI options and said price is too high.",
		},
	})

	assert.Contains(t, out, "## PREVIOUS CALLS WITH THIS CUSTOMER")
	assert.Contains(t, out, "2026-09-01")
	assert.Contains(t, out, "EMI options")
	assert.NotContains(t, out, "What went wrong")
	assert.NotContains(t, out, "Do better this time")
	// Guardrail instruction must be present so the agent never speaks the notes.
	assert.Contains(t, out, "never speak")
	// Off-topic details from dirty notes must be ignored.
	assert.Contains(t, out, "unrelated to the product")
}

// The memory block must precede the CALL FLOW questionnaire: confirmed
// facts established before the script beats an afterthought at the end.
// With memory + a product questionnaire, the questionnaire must be replaced
// by the confirm-and-book flow — not rendered alongside it.
func TestDefaultPromptPlacesMemoryBeforeCallFlow(t *testing.T) {
	out := buildDefaultPrompt(promptContext{
		Language:             "en",
		CallMemory:           renderCallMemory([]db.CallMemory{{CreatedAt: "2026-09-03", Summary: "15 users, Bengaluru"}}),
		CompanyName:          "ACME",
		CallFlowInstructions: "QUESTIONNAIRE: ask how many users, which sector, how many locations.",
	})

	memIdx := strings.Index(out, "## PREVIOUS CALLS WITH THIS CUSTOMER")
	flowIdx := strings.Index(out, "## CALL FLOW")
	assert.True(t, memIdx > 0, "memory block missing")
	assert.True(t, flowIdx > memIdx, "memory block must come before CALL FLOW")
	assert.Contains(t, out, "CONTINUATION CALL")
	assert.NotContains(t, out, "QUESTIONNAIRE", "questionnaire must be replaced when memory exists")
}

// Without memory the product questionnaire stays untouched.
func TestDefaultPromptWithoutMemoryKeepsQuestionnaire(t *testing.T) {
	out := buildDefaultPrompt(promptContext{
		Language:             "en",
		CompanyName:          "ACME",
		CallFlowInstructions: "QUESTIONNAIRE: ask how many users, which sector, how many locations.",
	})

	assert.NotContains(t, out, "## PREVIOUS CALLS WITH THIS CUSTOMER")
	assert.NotContains(t, out, "CONTINUATION CALL")
	assert.Contains(t, out, "QUESTIONNAIRE")
}

func TestDefaultPromptHandlesSemanticRepeatsInMainStreamingTurn(t *testing.T) {
	out := buildDefaultPrompt(promptContext{
		Language:    "te",
		CompanyName: "ACME",
	})

	// Repeat semantics belong in the main model request, which already has the
	// full multilingual conversation history. This prevents a separate blocking
	// classifier request from being required on the realtime voice path.
	assert.Contains(t, out, "REPEATED CUSTOMER QUESTIONS")
	assert.Contains(t, out, "same question repeatedly")
	assert.Contains(t, out, "simpler words")
	assert.Contains(t, out, "third total ask")
	assert.Contains(t, out, "fourth total ask")
}

func TestOtherLeadSourceUsesGenericAdEnquiryInEveryLanguage(t *testing.T) {
	tests := map[string]string{
		"en": "see our ad and enquire",
		"hi": "हमारा ad देखकर enquiry की थी",
		"mr": "आमची ad बघून enquiry केली होती",
		"bn": "আমাদের ad দেখে enquiry করেছিলেন",
		"gu": "અમારી ad જોઈને enquiry કરી હતી",
		"pa": "ਸਾਡਾ ad ਵੇਖ ਕੇ enquiry ਕੀਤੀ ਸੀ",
		"ta": "எங்கள் ad பார்த்து enquiry செய்திருந்தீர்கள்",
		"te": "మా ad చూసి enquiry చేశారు",
		"kn": "ನಮ್ಮ ad ನೋಡಿ enquiry ಮಾಡಿದ್ದೀರಿ",
		"ml": "ഞങ്ങളുടെ ad കണ്ട് enquiry ചെയ്തിരുന്നു",
	}

	for language, expected := range tests {
		t.Run(language, func(t *testing.T) {
			assert.Equal(t, expected, sourceContextInline("other", language))
			assert.NotContains(t, strings.ToLower(buildGreeting("Sri", "GlobusCRM", "Aditya", "calling", "other", language)), "other")
		})
	}
	assert.Equal(t, "other", canonicalSource("Others"))
	assert.Equal(t, "other", canonicalSource("other source"))
}

func TestRenderCallMemoryOmitsEmptyFields(t *testing.T) {
	out := renderCallMemory([]db.CallMemory{
		{CreatedAt: "2026-09-02", Summary: "No answer details"},
	})

	assert.Contains(t, out, "What happened: No answer details")
	assert.NotContains(t, out, "What went wrong:")
	assert.NotContains(t, out, "Do better this time:")
}

func TestRenderCallMemoryCapsFieldLength(t *testing.T) {
	long := strings.Repeat("x", 500)
	out := renderCallMemory([]db.CallMemory{
		{CreatedAt: "2026-09-03", Summary: long},
	})

	// Field is truncated to callMemoryMaxFieldLen + ellipsis.
	assert.Contains(t, out, strings.Repeat("x", callMemoryMaxFieldLen)+"…")
	assert.NotContains(t, out, strings.Repeat("x", callMemoryMaxFieldLen+1))
}

func TestClampRunes(t *testing.T) {
	assert.Equal(t, "hello", clampRunes("hello", 10))
	assert.Equal(t, "hello", clampRunes("  hello  ", 10))
	assert.Equal(t, "hell…", clampRunes("hello world", 4))
	assert.Equal(t, "", clampRunes("   ", 4))
}
