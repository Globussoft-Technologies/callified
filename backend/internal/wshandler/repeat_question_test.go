package wshandler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRepeatedQuestionInstructionEscalatesThenCloses(t *testing.T) {
	sess := NewCallSession("web_sim_repeat", nil, zap.NewNop())

	first := sess.RepeatedQuestionDecision("What company is this?")
	assert.Empty(t, first.Instruction)
	assert.True(t, first.AllowHangup)

	second := sess.RepeatedQuestionDecision("what company is this")
	assert.Contains(t, second.Instruction, "2nd total time")
	assert.False(t, second.AllowHangup)

	third := sess.RepeatedQuestionDecision("What company is this?")
	assert.Contains(t, third.Instruction, "3 total time")
	assert.Contains(t, third.Instruction, "senior teammate should call back")
	assert.Contains(t, third.Instruction, "Do not include [HANGUP]")
	assert.Contains(t, third.Instruction, "do not say you already answered")
	assert.False(t, third.AllowHangup)

	fourth := sess.RepeatedQuestionDecision("What company is this?")
	assert.Contains(t, fourth.Instruction, "4 total time")
	assert.Contains(t, fourth.Instruction, "senior teammate will follow up")
	assert.Contains(t, fourth.Instruction, "do not say you already answered")
	assert.Contains(t, fourth.Instruction, "end with [HANGUP]")
	assert.True(t, fourth.AllowHangup)
}

func TestRepeatedQuestionInstructionIgnoresDifferentQuestions(t *testing.T) {
	sess := NewCallSession("web_sim_repeat", nil, zap.NewNop())

	assert.Empty(t, sess.RepeatedQuestionDecision("What company is this?").Instruction)
	assert.Empty(t, sess.RepeatedQuestionDecision("What product do you sell?").Instruction)
}

func TestNormalizeQuestionTextKeepsUnicodeLetters(t *testing.T) {
	normalized := normalizeQuestionText("  హలో, ఎవరు మాట్లాడుతున్నారు?  ")

	assert.True(t, strings.Contains(normalized, "హలో"))
	assert.NotContains(t, normalized, "?")
}
