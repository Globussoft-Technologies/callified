package api

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCampaignActivityFilterFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/?search=Jane+Doe&from=2026-09-01T09%3A30&to=2026-09-22T18%3A45", nil)
	filter, err := campaignActivityFilterFromRequest(req)

	require.NoError(t, err)
	require.Equal(t, "Jane Doe", filter.Search)
	require.Equal(t, "2026-09-01 09:30:00", filter.From)
	require.Equal(t, "2026-09-22 18:45:00", filter.To)
}

func TestCampaignActivityFilterRejectsInvalidRange(t *testing.T) {
	req := httptest.NewRequest("GET", "/?from=2026-09-22T18%3A45&to=2026-09-01T09%3A30", nil)
	_, err := campaignActivityFilterFromRequest(req)
	require.ErrorContains(t, err, "from date must not be after to date")
}

func TestNormalizeActivityDateConvertsOffsetToUTC(t *testing.T) {
	got, err := normalizeActivityDate("2026-09-22T12:00:00+05:30")
	require.NoError(t, err)
	require.Equal(t, "2026-09-22 06:30:00", got)
}
