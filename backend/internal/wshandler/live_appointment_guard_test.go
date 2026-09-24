package wshandler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/globussoft/callified-backend/internal/llm"
)

func TestLiveAppointmentGuardClosesAfterConfirmedSlotAndAcknowledgement(t *testing.T) {
	guard := newLiveAppointmentGuard()
	history := []llm.ChatMessage{
		{Role: "user", Text: "రేపు మధ్యాహ్నం రెండు గంటలకు సరే"},
	}
	require.True(t, guard.ObserveAgentConfirmation("మీ డెమో రేపు మధ్యాహ్నం రెండు గంటలకు ఖరారైంది.", history))
	assert.True(t, guard.ObserveCustomerAcknowledgement("Okay.", history))
	assert.True(t, guard.ShouldHoldOutput())

	guard.MarkFinalPromptSent()
	assert.False(t, guard.ShouldHoldOutput())
}

func TestLiveAppointmentGuardRequiresCustomerDateAndTime(t *testing.T) {
	guard := newLiveAppointmentGuard()
	history := []llm.ChatMessage{{Role: "user", Text: "Tell me about the product"}}
	assert.False(t, guard.ObserveAgentConfirmation("Your demo is tomorrow at two PM.", history))
	assert.False(t, guard.ObserveCustomerAcknowledgement("Okay", history))
	assert.False(t, guard.ShouldHoldOutput())
}

func TestPositiveAppointmentAcknowledgementIsStrict(t *testing.T) {
	for _, text := range []string{"Okay", "OK.", "సరే", "ಹೌದು", "ठीक है", "சரி"} {
		assert.True(t, isPositiveAppointmentAcknowledgement(text), text)
	}
	for _, text := range []string{"Okay, but not tomorrow", "No", "Hello", "Tell me again"} {
		assert.False(t, isPositiveAppointmentAcknowledgement(text), text)
	}
}
