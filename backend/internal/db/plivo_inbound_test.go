package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestEnsurePlivoInboundCallRetriesAndScope(t *testing.T) {
	for _, tc := range []struct {
		name, status, phone string
		org                 int64
		missing, fail       bool
	}{
		{name: "new incoming call", missing: true},
		{name: "answer retry", org: 7, status: "in-progress", phone: "+919123456789"},
		{name: "completed call cannot reopen", org: 7, status: "completed", phone: "+919123456789", fail: true},
		{name: "other context rejected", org: 8, status: "in-progress", phone: "+919123456789", fail: true},
		{name: "caller cannot change on retry", org: 7, status: "in-progress", phone: "+919999999999", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer pool.Close()
			d := &DB{pool: pool}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM org_exotel_accounts.*FOR UPDATE").WithArgs(int64(3), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
			lookup := mock.ExpectQuery("SELECT org_id, provider, status,.*call_logs WHERE call_sid=\\? AND org_id=\\?").WithArgs("plivo_in_3_call", int64(7))
			if tc.missing {
				lookup.WillReturnError(sql.ErrNoRows)
				mock.ExpectExec("INSERT INTO call_logs").WithArgs(int64(7), "plivo_in_3_call", "+919123456789").WillReturnResult(sqlmock.NewResult(1, 1))
			} else {
				lookup.WillReturnRows(sqlmock.NewRows([]string{"org_id", "provider", "status", "phone"}).AddRow(tc.org, "plivo-inbound", tc.status, tc.phone))
			}
			if tc.fail {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = d.EnsurePlivoInboundCall(context.Background(), 3, 7, "plivo_in_3_call", "+919123456789")
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInboundStatusIsScopedAndCannotOverwriteTerminalStatus(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE call_logs SET status=\\? WHERE org_id=\\? AND call_sid=\\? AND provider='plivo-inbound' AND status='in-progress'").
		WithArgs("completed", int64(7), "plivo_in_3_call").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdatePlivoInboundStatus(context.Background(), 7, "plivo_in_3_call", "completed"))
	require.NoError(t, mock.ExpectationsWereMet())
}
