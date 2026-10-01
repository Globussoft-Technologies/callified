package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemDispositionForCallStatus(t *testing.T) {
	code, _, ok := SystemDispositionForCallStatus("no-answer")
	require.True(t, ok)
	require.Equal(t, "no_answer", code)

	code, _, ok = SystemDispositionForCallStatus("busy")
	require.True(t, ok)
	require.Equal(t, "callback", code)

	_, _, ok = SystemDispositionForCallStatus("completed")
	require.False(t, ok)
}

func TestNormalizeDispositionOption(t *testing.T) {
	option, err := normalizeDispositionOption(DispositionOption{Code: " HOT_Lead ", Label: " Hot lead "}, 0)
	require.NoError(t, err)
	require.Equal(t, "hot_lead", option.Code)
	require.Equal(t, "Hot lead", option.Label)
	require.Equal(t, "#64748b", option.Color)
	require.Equal(t, 10, option.SortOrder)
}
