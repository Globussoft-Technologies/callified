package wshandler

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIncomingInstructionsReplaceFixedBusinessRules(t *testing.T) {
	prompt := buildInboundInstructions("Hindi", "Ask for email. Explain the Diwali discount. Do not request a phone number.", "Career counselling in Nagpur")
	require.Contains(t, prompt, "Ask for email.")
	require.Contains(t, prompt, "Career counselling in Nagpur")
	require.NotContains(t, prompt, "EmpMonitor")
	require.NotContains(t, prompt, "Phone number is compulsory")
	require.NotContains(t, prompt, "Do not ask for email")
}
