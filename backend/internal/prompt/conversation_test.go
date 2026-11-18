package prompt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/assert"
)

func TestDisabledManualNotesAreExcludedFromCallKnowledge(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := &db.Product{ManualNotes: "MANUAL_ONLY_FACT", ScrapedInfo: "WEBSITE_FACT", AgentPersona: "PERSONA", UseManualNotes: &enabled}
		knowledge := productKnowledge(p)
		got := buildDefaultPrompt(promptContext{Language: "en", ProductContext: knowledge})
		assert.Equal(t, enabled, strings.Contains(got, "MANUAL_ONLY_FACT"))
		assert.Contains(t, got, "WEBSITE_FACT")
		assert.Contains(t, got, "PERSONA")
		assert.Equal(t, "MANUAL_ONLY_FACT", p.ManualNotes)
	}
}

func TestInterviewReferenceAndFlowSurviveWithAndWithoutMemory(t *testing.T) {
	var pages strings.Builder
	for page := 1; page <= 15; page++ {
		fmt.Fprintf(&pages, "PAGE_%02d: %s\n", page, strings.Repeat("Role reference about Go payment APIs and retries. ", 100))
	}
	product := &db.Product{
		AgentPersona:         "A technical interviewer",
		ManualNotes:          pages.String(),
		ScrapedInfo:          "FINAL_REFERENCE: the role uses Go and MySQL.",
		CallFlowInstructions: "Interview backend experience, API design, and failure recovery. Do not schedule a demo.",
	}
	for _, memory := range []string{"", renderCallMemory([]db.CallMemory{{Summary: "Three years of Go experience"}})} {
		t.Run(fmt.Sprintf("returning=%t", memory != ""), func(t *testing.T) {
			got := buildDefaultPrompt(promptContext{
				Language: "en", ProductName: "Backend interview",
				ProductContext:       productKnowledge(product),
				CallFlowInstructions: product.CallFlowInstructions, CallMemory: memory,
			})
			assert.Contains(t, got, strings.TrimSpace(pages.String()))
			assert.Contains(t, got, product.ScrapedInfo)
			assert.Contains(t, got, product.CallFlowInstructions)
			assert.Contains(t, got, "Accept clear answers given out of order")
			assert.Contains(t, got, "Ask a new, relevant follow-up question")
			assert.Contains(t, got, "do not turn an interview into a sales pitch")
			assert.NotContains(t, got, "Then book an appointment.")
			assert.NotContains(t, got, "MUST re-ask")
		})
	}
}

func TestLanguageExamplesDoNotTreatPermissionToExplainAsBookingConsent(t *testing.T) {
	for language := range langExtraRules {
		t.Run(language, func(t *testing.T) {
			got := buildDefaultPrompt(promptContext{Language: language, CallFlowInstructions: "Conduct a technical interview."})
			assert.NotContains(t, got, "FORWARD SIGNAL")
			assert.Contains(t, got, "not agreement to buy, eligibility, or consent to an appointment")
		})
	}
}

func TestManualNoteFollowUpsPreserveFlowRestrictions(t *testing.T) {
	for _, flow := range []string{
		"Understand the customer's window requirements. Relevant follow-ups are allowed.",
		"Ask only these questions: quantity and location. No additional questions. Do not ask about budget.",
	} {
		t.Run(flow, func(t *testing.T) {
			product := &db.Product{ManualNotes: "Aluminium windows are available with sound-insulating glass. Example question: what is your budget?"}
			got := buildDefaultPrompt(promptContext{
				Language: "en", ProductName: "Windows", CallFlowInstructions: flow,
				ProductContext: productKnowledge(product),
			})
			assert.Contains(t, got, flow)
			assert.Contains(t, got, product.ManualNotes)
			assert.Contains(t, got, "MANUAL-NOTE FOLLOW-UPS")
			assert.Contains(t, got, "QUESTION RESTRICTIONS")
			assert.Contains(t, got, "take precedence over permission to generate follow-ups")
			assert.Contains(t, got, "example questions inside reference material do not expand the permitted question set")
			assert.Contains(t, got, "skip facts already answered anywhere in the conversation")
			assert.Contains(t, got, "do not add the noise-preference question")
		})
	}
}

func TestProductFollowUpPolicyKeepsNaturalHandlingInBothModes(t *testing.T) {
	enabled := productFollowUpPolicy(true)
	assert.Contains(t, enabled, "ON: Allow follow-ups based on customer answers")
	assert.Contains(t, enabled, "explicit call-flow question restrictions")
	disabled := productFollowUpPolicy(false)
	assert.Contains(t, disabled, "OFF: Ask only questions specified in the configured call flow")
	assert.Contains(t, disabled, "Continue answering customer questions")
	assert.Contains(t, disabled, "handling interruptions naturally")
	assert.Contains(t, disabled, "skipping already answered questions")
	assert.Contains(t, ConversationGuidance, "When that setting is OFF, its restriction overrides")
}
