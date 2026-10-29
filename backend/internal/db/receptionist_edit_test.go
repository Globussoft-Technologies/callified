package db

import (
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReceptionistEditUpdatesMatchedCRMName(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE.*FOR UPDATE").WithArgs(int64(100), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"lead_id"}).AddRow(10))
	mock.ExpectQuery("SELECT id FROM leads WHERE org_id=.*FOR UPDATE").WithArgs(int64(7), "9876543210", "919876543210", int64(10)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectExec("UPDATE leads SET first_name=\\?,last_name=\\? WHERE id=\\? AND org_id=\\?").WithArgs("Parent", "", int64(20), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE call_transcripts SET inbound_first_name").WithArgs("Parent", nil, "9876543210", nil, "new", int64(20), int64(100), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	id, err := d.SaveReceptionistCallDetails(context.Background(), 7, 100, "Parent", "", "9876543210", "", "new")
	require.NoError(t, err)
	require.EqualValues(t, 20, id)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestReceptionistEditRejectsTranscriptOutsideOrganization(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE.*FOR UPDATE").WithArgs(int64(100), int64(8)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err = d.SaveReceptionistCallDetails(context.Background(), 8, 100, "Parent", "", "9876543210", "", "new")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReceptionistEditNameOnlyUpdatesLinkedLead(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE.*FOR UPDATE").WithArgs(int64(101), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"lead_id"}).AddRow(20))
	mock.ExpectExec("UPDATE leads SET first_name").WithArgs("New", "Name", int64(20), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE call_transcripts SET inbound_first_name").WithArgs("New", "Name", nil, nil, "new", int64(20), int64(101), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	_, err = d.SaveReceptionistCallDetails(context.Background(), 7, 101, "New", "Name", "", "", "new")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
