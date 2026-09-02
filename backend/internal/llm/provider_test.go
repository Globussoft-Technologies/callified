package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeRepeatIntentKey(t *testing.T) {
	assert.Equal(t, "ask_price", sanitizeRepeatIntentKey("`Ask Price.`"))
	assert.Equal(t, "ask_biometrics_meaning", sanitizeRepeatIntentKey("ask biometrics meaning"))
	assert.Empty(t, sanitizeRepeatIntentKey("none"))
	assert.Empty(t, sanitizeRepeatIntentKey("Greeting."))
}
