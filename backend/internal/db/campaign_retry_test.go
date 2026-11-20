package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCampaignAutomaticRetriesCanBeExplicitlySaved(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	for _, enabled := range []bool{false, true, false} {
		mock.ExpectExec("UPDATE campaigns SET auto_retry_enabled").WithArgs(enabled, int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, d.UpdateCampaign(3, "", "", "", "", 0, &enabled))
	}
	require.NoError(t, d.UpdateCampaign(3, "", "", "", "", 0, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestRetryQueueRequiresMatchingEnabledCampaign(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	next := time.Now()
	mock.ExpectExec("INSERT INTO call_retries.*SELECT.*FROM campaigns WHERE id=\\? AND org_id=\\? AND auto_retry_enabled=1").WithArgs(int64(2), int64(3), int64(4), 3, next, int64(3), int64(4)).WillReturnResult(sqlmock.NewResult(0, 0))
	id, err := d.CreateRetry(2, 3, 4, 3, next)
	require.NoError(t, err)
	require.Zero(t, id)
	mock.ExpectQuery("SELECT EXISTS.*campaigns WHERE id=\\? AND org_id=\\? AND auto_retry_enabled=1").WithArgs(int64(3), int64(4)).WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))
	enabled, err := d.CampaignAutoRetriesEnabled(4, 3)
	require.NoError(t, err)
	require.False(t, enabled)
	mock.ExpectQuery("SELECT id, lead_id,.*EXISTS.*c.id=call_retries.campaign_id AND c.org_id=call_retries.org_id AND c.auto_retry_enabled=1").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rows, err := d.GetPendingRetries()
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateCampaignSavesAutomaticRetryChoice(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		pool, mock, err := sqlmock.New()
		require.NoError(t, err)
		d := &DB{pool: pool}
		mock.ExpectExec("INSERT INTO campaigns .*auto_retry_enabled").WithArgs(int64(4), int64(2), "Campaign", "instagram", "voice", int64(5), enabled).WillReturnResult(sqlmock.NewResult(7, 1))
		id, err := d.CreateCampaign(4, 2, "Campaign", "instagram", "voice", 5, enabled)
		require.NoError(t, err)
		require.Equal(t, int64(7), id)
		require.NoError(t, mock.ExpectationsWereMet())
		pool.Close()
	}
}
