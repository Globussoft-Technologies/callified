package db

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTataSecretRotationScopesOrganization(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE org_exotel_accounts SET tata_stream_secret_hash").WithArgs("new-hash", int64(7), int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	require.Error(t, d.SetTataStreamSecret(7, 9, "new-hash"))
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestTataSecretLookupResolvesServerOwnership(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT id,org_id").WithArgs("known-hash").WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "direction", "caller_id", "provider"}).AddRow(7, 9, "inbound", "919999999999", "tata"))
	account, err := d.GetTataAccountByStreamSecret(context.Background(), "known-hash")
	require.NoError(t, err)
	require.Equal(t, int64(9), account.OrgID)
	require.Equal(t, int64(7), account.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTataReadinessRequiresVerifiedCurrentSecret(t *testing.T) {
	for _, ready := range []bool{false, true} {
		pool, mock, err := sqlmock.New()
		require.NoError(t, err)
		d := &DB{pool: pool}
		mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(7), int64(9)).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(ready))
		actual, err := d.TataStreamReady(context.Background(), 7, 9)
		require.NoError(t, err)
		require.Equal(t, ready, actual)
		require.NoError(t, mock.ExpectationsWereMet())
		pool.Close()
	}
}
func TestTataVerificationCannotApproveRotatedSecret(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE org_exotel_accounts SET tata_stream_verified_hash").WithArgs("old", int64(7), int64(9), "old").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COALESCE").WithArgs("old", "old", int64(7), int64(9)).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(false))
	require.Error(t, d.MarkTataStreamVerified(context.Background(), 7, 9, "old"))
	require.NoError(t, mock.ExpectationsWereMet())
}
