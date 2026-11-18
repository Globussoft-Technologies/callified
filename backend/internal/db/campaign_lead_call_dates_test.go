package db

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCampaignLeadCallDatesPreserveCampaignOrgAndOneSidedRanges(t *testing.T) {
	for _, filter := range []CampaignLeadsFilter{
		{CampaignID: 7, CalledFrom: "2026-11-01 00:00:00"},
		{CampaignID: 7, CalledTo: "2026-11-17 12:00:00"},
		{CampaignID: 7, CalledFrom: "2026-11-01 00:00:00", CalledTo: "2026-11-17 12:00:00"},
	} {
		clause, args := campaignLeadCallDateClause(filter)
		require.Contains(t, clause, "EXISTS (SELECT 1 FROM call_logs")
		require.Contains(t, clause, "OR EXISTS (SELECT 1 FROM call_transcripts")
		require.Contains(t, clause, "call_date.org_id=l.org_id AND call_date.lead_id=l.id AND call_date.campaign_id=?")
		require.NotContains(t, clause, "scheduled_at")
		require.Equal(t, int64(7), args[0])
		if filter.CalledFrom == "" {
			require.NotContains(t, clause, "created_at>=?")
		}
		if filter.CalledTo == "" {
			require.NotContains(t, clause, "created_at<=?")
		}
	}
	clause, args := campaignLeadCallDateClause(CampaignLeadsFilter{CampaignID: 7})
	require.Empty(t, clause)
	require.Empty(t, args)
}

func TestCampaignLeadCallDatesAppliedToBothRowsAndPaginationTotal(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	filter := CampaignLeadsFilter{CampaignID: 7, CalledFrom: "2026-11-01 00:00:00", CalledTo: "2026-11-17 12:00:00"}
	clause, _ := campaignLeadCallDateClause(filter)
	mock.ExpectQuery("SELECT l.id,.*"+regexp.QuoteMeta(clause)+".*LIMIT").
		WithArgs(int64(7), "", "", int64(7), int64(7), "%%", "%%", "%%", "%%", "%%",
			int64(7), filter.CalledFrom, filter.CalledTo, int64(7), filter.CalledFrom, filter.CalledTo, int64(100), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err = d.GetCampaignLeadsPaginated(filter, 100, 0)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT COUNT.*"+regexp.QuoteMeta(clause)).
		WithArgs(int64(7), "", "", int64(7), "%%", "%%", "%%", "%%", "%%",
			int64(7), filter.CalledFrom, filter.CalledTo, int64(7), filter.CalledFrom, filter.CalledTo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	total, err := d.CountCampaignLeads(filter)
	require.NoError(t, err)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
