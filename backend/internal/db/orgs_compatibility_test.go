package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSystemPromptModePreservesLegacyReplaceDefault(t *testing.T) {
	assert.Equal(t, "replace", normalizeSystemPromptMode(""))
	assert.Equal(t, "replace", normalizeSystemPromptMode("unknown"))
	assert.Equal(t, "extend", normalizeSystemPromptMode(" Extend "))
}

func TestPreferOrganizationPronunciationsOverridesGlobalRule(t *testing.T) {
	candidates := []Pronunciation{
		{ID: 2, OrgID: 7, Word: "Claude", Phonetic: "Cloud", Inherited: false},
		{ID: 1, OrgID: 0, Word: "CLAUDE", Phonetic: "C L A U D E", Inherited: true},
		{ID: 3, OrgID: 0, Word: "Callified", Phonetic: "Call-i-fied", Inherited: true},
	}

	got := preferOrganizationPronunciations(candidates)
	assert.Len(t, got, 2)
	assert.Equal(t, int64(2), got[0].ID)
	assert.False(t, got[0].Inherited)
	assert.Equal(t, int64(3), got[1].ID)
	assert.True(t, got[1].Inherited)
}
