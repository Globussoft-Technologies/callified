package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCampaignActivityFilterClause(t *testing.T) {
	clause, args := campaignActivityFilterClause(CampaignActivityFilter{
		Search: "  Jane Doe  ",
		From:   "2026-09-01 09:00:00",
		To:     "2026-09-22 18:00:00",
	}, "ct.created_at")

	require.Contains(t, clause, "CONCAT_WS")
	require.Contains(t, clause, "ct.created_at >= ?")
	require.Contains(t, clause, "ct.created_at <= ?")
	require.Equal(t, []any{
		"%Jane Doe%", "%Jane Doe%", "%Jane Doe%", "%Jane Doe%",
		"2026-09-01 09:00:00", "2026-09-22 18:00:00",
	}, args)
}

func TestCampaignActivityFilterClauseEmpty(t *testing.T) {
	clause, args := campaignActivityFilterClause(CampaignActivityFilter{}, "r.created_at")
	require.Empty(t, clause)
	require.Empty(t, args)
}
