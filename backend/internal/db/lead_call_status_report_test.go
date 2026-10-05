package db

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLeadCallStatusWhereIncludesEveryFilter(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	where, args := leadCallStatusWhere(12, LeadCallStatusFilter{
		From: &from, To: &to, LeadID: 44,
		CampaignIDs: []int64{7, 8}, AgentUserIDs: []int64{9},
		Dispositions: []string{"interested", "callback"}, Search: "Asha",
	}, true)

	require.Contains(t, where, "cd.org_id=?")
	require.Contains(t, where, "cd.lead_id=?")
	require.Contains(t, where, "cd.campaign_id IN (?,?)")
	require.Contains(t, where, "cd.agent_user_id IN (?)")
	require.Contains(t, where, reportEffectiveDisposition+" IN (?,?)")
	require.Contains(t, where, "l.first_name LIKE ?")
	require.Len(t, args, 13)
	require.Equal(t, int64(12), args[0])
	require.Equal(t, "%Asha%", args[len(args)-1])
}

func TestLeadCallStatusWhereOmitsEmptyOptionalFilters(t *testing.T) {
	where, args := leadCallStatusWhere(5, LeadCallStatusFilter{}, false)
	require.Equal(t, " WHERE cd.org_id=? AND cd.lead_id IS NOT NULL", where)
	require.Equal(t, []any{int64(5)}, args)
	require.False(t, strings.Contains(where, " IN ("))
}

func TestLeadCallStatusWhereDefersDispositionForAggregateReport(t *testing.T) {
	where, args := leadCallStatusWhere(5, LeadCallStatusFilter{Dispositions: []string{"interested"}}, false)
	require.NotContains(t, where, reportEffectiveDisposition+" IN")
	require.Equal(t, []any{int64(5)}, args)
}
