package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCompleteCallingLeadScopesLatestCall(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE leads SET status=.*LOWER\\(status\\)='calling'.*SELECT MAX\\(id\\).*SELECT MAX\\(id\\)").
		WithArgs("Follow-up", int64(7), int64(9), int64(7), int64(9), int64(7), int64(9), "call-1").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.CompleteCallingLead(7, 9, "call-1", "Follow-up"))
	require.NoError(t, d.CompleteCallingLead(0, 9, "call-1", "Follow-up"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLeadStatusForCallDisposition(t *testing.T) {
	for code, expected := range map[string]string{"interested": "Interested", "not_interested": "Not Interested", "callback": "Follow-up", "appointment_booked": "Appointment Set", "no_answer": "New", "custom": "Contacted"} {
		require.Equal(t, expected, leadStatusForCallDisposition(code))
	}
}
