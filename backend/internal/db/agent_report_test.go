package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAgentReportAcceptsFractionalCallDurations(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	from := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(24*time.Hour - time.Second)
	mock.ExpectQuery("SELECT.*FROM users u").WithArgs(int64(7), from, to, int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "email", "name", "role", "calls", "connected", "completed", "unanswered", "busy", "failed", "talk", "idle", "break", "updates", "notes", "appointments", "conversions"}).
			AddRow(3, "admin@example.com", "Admin", "Admin", 1, 1, 1, 0, 0, 0, 173.4131851196289, 1.2, 2.8, 0, 0, 0, 0))
	rows, err := d.GetAgentActivitySummary(7, from, to, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 173, rows[0].TotalTalkTime)
	require.EqualValues(t, 1, rows[0].IdleTime)
	require.EqualValues(t, 3, rows[0].BreakTime)
	require.Equal(t, "Admin", rows[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}
