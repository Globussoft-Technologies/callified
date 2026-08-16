package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestApplyGuardrailsWithState(t *testing.T) {
	log := zap.NewNop()

	t.Run("preserves HANGUP", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Thanks, bye [HANGUP]", "en", false, log)
		assert.Equal(t, "Thanks, bye [HANGUP]", got)
	})

	t.Run("strips disallowed bracket tags", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Please [HOLD] wait a moment [MUTE]", "en", false, log)
		assert.Equal(t, "Please wait a moment", got)
	})

	t.Run("strips markdown", func(t *testing.T) {
		got := ApplyGuardrailsWithState("**Important** note: *do not* share `code`", "en", false, log)
		assert.NotContains(t, got, "**")
		assert.NotContains(t, got, "*")
		assert.NotContains(t, got, "`")
	})

	t.Run("redacts URLs and emails", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Visit https://example.com or email foo@bar.com", "en", false, log)
		assert.Contains(t, got, "[link removed]")
		assert.NotContains(t, got, "https://example.com")
		assert.NotContains(t, got, "foo@bar.com")
	})

	t.Run("redacts phone numbers", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Call me at 98765 43210", "en", false, log)
		assert.Contains(t, got, "[phone number]")
	})

	t.Run("strips repeated English greeting when greetingDone", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Hello, how can I help you?", "en", true, log)
		assert.Equal(t, "how can I help you?", got)
	})

	t.Run("strips repeated Hindi greeting when greetingDone", func(t *testing.T) {
		got := ApplyGuardrailsWithState("नमस्ते, आप कैसे हैं?", "hi", true, log)
		assert.Equal(t, "आप कैसे हैं?", got)
	})

	t.Run("keeps greeting when not done", func(t *testing.T) {
		got := ApplyGuardrailsWithState("Hello, how can I help you?", "en", false, log)
		assert.Equal(t, "Hello, how can I help you?", got)
	})

	t.Run("enforces English length limit", func(t *testing.T) {
		long := strings.Repeat("word ", 50) + " [HANGUP]"
		got := ApplyGuardrailsWithState(long, "en", false, log)
		assert.True(t, len(got) <= 220, "got length %d", len(got))
		assert.True(t, strings.HasSuffix(got, "[HANGUP]"))
	})

	t.Run("enforces Indic length limit", func(t *testing.T) {
		long := strings.Repeat("शब्द ", 60) + " [HANGUP]"
		got := ApplyGuardrailsWithState(long, "hi", false, log)
		assert.True(t, len(got) <= 260, "got length %d", len(got))
		assert.True(t, strings.HasSuffix(got, "[HANGUP]"))
	})
}
