package db

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReceptionistSettingsAreScopedAndEmptyWhenUnconfigured(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT greeting, instructions FROM org_receptionist_settings WHERE org_id = \\?").WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	settings, err := d.GetReceptionistSettings(7)
	require.NoError(t, err)
	require.Empty(t, settings.Instructions)
	mock.ExpectExec("INSERT INTO org_receptionist_settings").WithArgs(int64(7), "Welcome", "Ask about careers").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, d.SaveReceptionistSettings(7, ReceptionistSettings{Greeting: "Welcome", Instructions: "Ask about careers"}))
	require.NoError(t, mock.ExpectationsWereMet())
}
