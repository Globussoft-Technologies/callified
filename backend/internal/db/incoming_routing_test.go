package db

import (
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIncomingRouteDefaultsToAIAndScopesDestination(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT r.mode").WithArgs(int64(7), int64(17)).WillReturnError(sql.ErrNoRows)
	route, err := d.GetIncomingRoute(7, 17)
	require.NoError(t, err)
	require.Equal(t, "ai", route.Mode)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM org_forwarding_numbers WHERE org_id=\\? AND id=\\? FOR UPDATE").WithArgs(int64(7), int64(20)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	require.Error(t, d.SaveIncomingRoute(7, 17, IncomingRoute{Mode: "phone", DestinationID: 20}))
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestIncomingRoutingRetryKeepsFirstDestination(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("INSERT INTO incoming_route_snapshots").WithArgs(int64(7), "call-1", "ai", "", "").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT mode,phone,destination_name FROM incoming_route_snapshots WHERE org_id=\\? AND call_sid=\\?").WithArgs(int64(7), "call-1").WillReturnRows(sqlmock.NewRows([]string{"mode", "phone", "destination_name"}).AddRow("phone", "+919876543210", "Sales"))
	route, err := d.SnapshotIncomingRoute(context.Background(), 7, "call-1", IncomingRoute{Mode: "ai"})
	require.NoError(t, err)
	require.Equal(t, "phone", route.Mode)
	require.Equal(t, "+919876543210", route.Phone)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestCannotDeleteSelectedForwardingNumber(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM org_forwarding_numbers").WithArgs(int64(7), int64(20)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery("SELECT COUNT").WithArgs(int64(7), int64(20)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	require.ErrorIs(t, d.DeleteForwardingNumber(7, 20), ErrForwardingInUse)
	require.NoError(t, mock.ExpectationsWereMet())
}
