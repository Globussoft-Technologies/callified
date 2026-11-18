package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUnchangedLeadUpdateSucceedsOnlyForAnExistingScopedLead(t *testing.T) {
	for _, exists := range []bool{true, false} {
		pool, mock, err := sqlmock.New()
		require.NoError(t, err)
		d := &DB{pool: pool}
		mock.ExpectExec(`UPDATE leads SET.*WHERE id=\? AND org_id=\?`).
			WithArgs("First", "Last", "9876543210", "manual", nil, "Company", nil, int64(9), int64(7)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS .*leads WHERE id=\? AND org_id=\?`).WithArgs(int64(9), int64(7)).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
		ok, err := d.UpdateLead(9, "First", "Last", "9876543210", "manual", "", "Company", 0, 7)
		require.NoError(t, err)
		require.Equal(t, exists, ok)
		require.NoError(t, mock.ExpectationsWereMet())
		pool.Close()
	}
}
