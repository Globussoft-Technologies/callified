package db

import (
	"database/sql"
	"errors"
	"math"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectCallCreditLock(mock sqlmock.Sqlmock, balance int64, rate int) {
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO org_credits").WithArgs(int64(7), DefaultRatePerMinPaise).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT balance_paise,.*FOR UPDATE").WithArgs(DefaultRatePerMinPaise, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "rate"}).AddRow(balance, rate))
}

func TestDeductCallCreditsRecordsFullCharge(t *testing.T) {
	for _, tc := range []struct {
		name            string
		balance, charge int64
		rate            int
		duration        float64
	}{
		{"enough credit", 1000, 500, 500, 60},
		{"exceeds credit", 100, 500, 500, 60},
		{"zero after concurrent call", 0, 500, 500, 60},
		{"already in debt", -400, 500, 500, 60},
		{"fractional second", 100, 9, 500, 1},
		{"invalid stored rate uses default", 100, 500, 0, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer pool.Close()
			d := &DB{pool: pool}
			expectCallCreditLock(mock, tc.balance, tc.rate)
			mock.ExpectQuery("SELECT id FROM credit_transactions WHERE org_id=.*reference=.*").WithArgs(int64(7), "call-1").WillReturnError(sql.ErrNoRows)
			mock.ExpectExec("UPDATE org_credits SET balance_paise = balance_paise -").WithArgs(tc.charge, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
			rate := tc.rate
			if rate <= 0 {
				rate = DefaultRatePerMinPaise
			}
			mock.ExpectExec("INSERT INTO credit_transactions").WithArgs(int64(7), -tc.charge, tc.balance-tc.charge, "call-1", tc.duration, rate).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			charge, balance, err := d.DeductCallCredits(7, "call-1", tc.duration)
			require.NoError(t, err)
			require.Equal(t, tc.charge, charge)
			require.Equal(t, tc.balance-tc.charge, balance)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeductCallCreditsDuplicateIsNoOpAfterLock(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	expectCallCreditLock(mock, -400, 500)
	mock.ExpectQuery("SELECT id FROM credit_transactions WHERE org_id=.*reference=.*").WithArgs(int64(7), "call-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	charge, balance, err := d.DeductCallCredits(7, "call-1", 60)
	require.NoError(t, err)
	require.Zero(t, charge)
	require.EqualValues(t, -400, balance)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductCallCreditsRollsBackWhenLedgerFails(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	expectCallCreditLock(mock, 100, 500)
	mock.ExpectQuery("SELECT id FROM credit_transactions").WithArgs(int64(7), "call-1").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE org_credits").WithArgs(int64(500), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO credit_transactions").WillReturnError(errors.New("ledger unavailable"))
	mock.ExpectRollback()
	_, _, err = d.DeductCallCredits(7, "call-1", 60)
	require.ErrorContains(t, err, "ledger unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCallingGateAfterRecharge(t *testing.T) {
	for _, balance := range []int64{-900, -1, 0, 1, 500} {
		pool, mock, err := sqlmock.New()
		require.NoError(t, err)
		d := &DB{pool: pool}
		mock.ExpectQuery("SELECT org_id, balance_paise,").WithArgs(DefaultRatePerMinPaise, int64(7)).
			WillReturnRows(sqlmock.NewRows([]string{"org", "balance", "rate", "updated"}).AddRow(7, balance, 500, "2026-11-03 12:00:00"))
		credit, err := d.GetOrgCredit(7)
		require.NoError(t, err)
		require.Equal(t, balance, credit.BalancePaise)
		require.Equal(t, balance > 0, credit.CanStartCall())
		require.GreaterOrEqual(t, credit.MinutesAvailable, 0)
		require.NoError(t, mock.ExpectationsWereMet())
		pool.Close()
	}
	var missing *OrgCredit
	require.False(t, missing.CanStartCall())
}

func TestAddCreditsPaysOutstandingBalance(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO org_credits").WithArgs(int64(7), DefaultRatePerMinPaise).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE org_credits SET balance_paise = balance_paise \\+").WithArgs(int64(300), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT balance_paise,").WithArgs(DefaultRatePerMinPaise, int64(7)).WillReturnRows(sqlmock.NewRows([]string{"balance", "rate"}).AddRow(-100, 500))
	mock.ExpectExec("INSERT INTO credit_transactions").WithArgs(int64(7), int64(300), int64(-100), "purchase", "payment-1", 500, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	_, err = d.AddCredits(7, 300, "purchase", "payment-1", "")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductCallCreditsRejectsNonFiniteDuration(t *testing.T) {
	d := &DB{}
	for _, duration := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, _, err := d.DeductCallCredits(7, "call-1", duration)
		require.Error(t, err)
	}
}
