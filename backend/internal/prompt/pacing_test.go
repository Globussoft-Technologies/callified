package prompt

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEveryLanguageAllowsCompleteAnswersAndDistinguishesPostponement(t *testing.T) {
	for lang := range langExtraRules {
		t.Run(lang, func(t *testing.T) {
			p := buildDefaultPrompt(promptContext{Language: lang, CallFlowInstructions: "Discuss the customer's requirements."})
			require.Contains(t, p, "A complete answer may stand alone")
			require.Contains(t, p, "A postponement is not a permanent rejection")
			require.Contains(t, p, "without requiring them to choose a callback time")
			require.NotContains(t, p, "1 follow-up")
			require.NotContains(t, p, "answer in ONE sentence")
			require.NotContains(t, p, "details senior")
		})
	}
}
