package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestForwardedHistoryIncludesDestinationWithoutTranscript(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT cl.id,.*").WithArgs(int64(7), 50).WillReturnRows(sqlmock.NewRows([]string{"id", "lead", "first", "last", "phone", "status", "created", "destination", "target"}).AddRow(12, 0, "", "", "+919876543210", "completed", "2026-10-28 12:00:00", "Sales", "+919123456789"))
	calls, err := d.GetRecentForwardedIncomingCalls(7, 50)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, "phone", calls[0].HandlingMode)
	require.Equal(t, "Sales", calls[0].ForwardedName)
	require.Zero(t, calls[0].TranscriptID)
	require.EqualValues(t, 12, calls[0].CallLogID)
	require.JSONEq(t, "[]", string(calls[0].Transcript))
	require.NoError(t, mock.ExpectationsWereMet())
}
