package prompt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/assert"
)

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
