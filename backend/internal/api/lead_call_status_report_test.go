package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseReportDateUsesExclusiveEnd(t *testing.T) {
	start, err := parseReportDate("2026-09-24", false)
	require.NoError(t, err)
	end, err := parseReportDate("2026-09-24", true)
	require.NoError(t, err)
	require.Equal(t, "2026-09-24", start.Format("2006-01-02"))
	require.Equal(t, "2026-09-25", end.Format("2006-01-02"))
}

func TestSpreadsheetSafePreventsFormulaInjection(t *testing.T) {
	for _, input := range []string{"=cmd()", "+123", "-2+3", "@SUM(A:A)", "  =1+1"} {
		require.Equal(t, "'"+input, spreadsheetSafe(input))
	}
	require.Equal(t, "Asha", spreadsheetSafe("Asha"))
}

func TestReportDuration(t *testing.T) {
	require.Equal(t, "01:01:02", reportDuration(3662))
}
