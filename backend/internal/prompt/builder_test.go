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

func TestOtherLeadSourceDoesNotClaimAdEnquiryInAnyLanguage(t *testing.T) {
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

	for language := range tests {
		t.Run(language, func(t *testing.T) {
			assert.Empty(t, sourceContextInline("other", language))
			greeting := strings.ToLower(buildGreeting("Sri", "GlobusCRM", "Aditya", "calling", "other", language))
			assert.NotContains(t, greeting, "enquir")
		})
	}
	assert.Equal(t, "Hi Sri. I'm Kore calling from dmitknowyourtalent.",
		buildGreeting("Sri", "dmitknowyourtalent", "Kore", "calling", "other", "en"))
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

func TestGeminiLiveVoiceUsesNamedPersona(t *testing.T) {
	tests := []struct {
		voice        string
		language     string
		wantName     string
		wantSpeaking string
	}{
		{voice: "Kore", language: "en", wantName: "Kore", wantSpeaking: "calling"},
		{voice: "kore", language: "hi", wantName: "Kore", wantSpeaking: "बोल रही हूँ"},
		{voice: "Puck", language: "hi", wantName: "Puck", wantSpeaking: "बोल रहा हूँ"},
	}

	for _, tt := range tests {
		t.Run(tt.voice+"_"+tt.language, func(t *testing.T) {
			name, speaking := agentIdentity(tt.voice, tt.language)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantSpeaking, speaking)
		})
	}
}

func TestUnknownOpaqueVoiceStillUsesSafeFallback(t *testing.T) {
	assert.Equal(t, "Arjun", AgentPersonaName("UnknownMixedCase123", "en"))
}

func TestAllGeminiVoiceIdentities(t *testing.T) {
	female := strings.Fields("Kore Aoede Leda Zephyr Callirrhoe Autonoe Despina Erinome Laomedeia Achernar Gacrux Pulcherrima Vindemiatrix Sulafat")
	male := strings.Fields("Puck Charon Fenrir Orus Enceladus Iapetus Umbriel Algieba Algenib Rasalgethi Alnilam Schedar Achird Zubenelgenubi Sadachbia Sadaltager")
	for _, group := range []struct {
		voices   []string
		speaking string
	}{
		{female, "बोल रही हूँ"}, {male, "बोल रहा हूँ"},
	} {
		for _, voice := range group.voices {
			for _, id := range []string{voice, strings.ToLower(voice)} {
				t.Run(id, func(t *testing.T) {
					assert.Equal(t, voice, AgentPersonaName(id, "en"))
					name, speaking := agentIdentity(id, "hi")
					assert.Equal(t, voice, name)
					assert.Equal(t, group.speaking, speaking)
				})
			}
		}
	}
}

func TestSelectOpeningGreeting(t *testing.T) {
	opening := "Namaste! NEET ya JEE? Kya aapke bachche ko apni right career direction pata hai?\nAajkal dasvi ke baad bahut se students NEET JEE ki coaching toh start kar dete hain, lekin kya unhe apni actual interests, strengths aur aptitude ke baare mein pata hota hai?"
	assert.Equal(t, opening, selectOpeningGreeting("  "+opening+"  ", "automatic introduction"))
	assert.Equal(t, "automatic introduction", selectOpeningGreeting(" \n ", "automatic introduction"))
}

func TestRenderProductOpeningGreeting(t *testing.T) {
	template := "नमस्ते {{customer_name}} जी। मैं {{agent_name}}, {{company_name}} से बोल रही हूँ।\nNEET ya JEE? Aajkal dasvi ke baad..."
	want := "नमस्ते sri जी। मैं Kore, KNOW YOUR TALENT से बोल रही हूँ।\nNEET ya JEE? Aajkal dasvi ke baad..."
	assert.Equal(t, want, RenderOpeningGreeting(template, "sri", "Kore", "KNOW YOUR TALENT"))
	assert.Equal(t, "{{agent_name}} Kore", RenderOpeningGreeting("{{customer_name}} {{agent_name}}", "{{agent_name}}", "Kore", "Company"))
	assert.Equal(t, "Exact literal paragraph", RenderOpeningGreeting("Exact literal paragraph", "sri", "Kore", "Company"))
}
