package db

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// This test creates and drops its own isolated schema on a disposable server.
func TestCallCreditsConcurrentMySQL(t *testing.T) {
	dsn := os.Getenv("CALLIFIED_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CALLIFIED_TEST_MYSQL_DSN is not configured")
	}
	cfg, err := mysql.ParseDSN(dsn)
	require.NoError(t, err)
	cfg.DBName = ""
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer admin.Close()
	name := fmt.Sprintf("callified_credit_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE `" + name + "`")
	require.NoError(t, err)
	defer admin.Exec("DROP DATABASE `" + name + "`")
	cfg.DBName = name
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer pool.Close()
	pool.SetMaxOpenConns(12)
	_, err = pool.Exec(`CREATE TABLE org_credits (org_id BIGINT PRIMARY KEY, balance_paise BIGINT NOT NULL DEFAULT 0, rate_per_min_paise INT DEFAULT 500, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB`)
	require.NoError(t, err)
	_, err = pool.Exec(`CREATE TABLE credit_transactions (id BIGINT AUTO_INCREMENT PRIMARY KEY, org_id BIGINT NOT NULL, delta_paise BIGINT NOT NULL, balance_after_paise BIGINT NOT NULL, type VARCHAR(50), reference VARCHAR(255), call_duration_s FLOAT, rate_per_min_paise INT, notes TEXT, INDEX(reference)) ENGINE=InnoDB`)
	require.NoError(t, err)
	_, err = pool.Exec(`INSERT INTO org_credits (org_id,balance_paise) VALUES (7,100),(8,100)`)
	require.NoError(t, err)
	d := &DB{pool: pool}
	const workers = 24
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := d.DeductCallCredits(7, fmt.Sprintf("call-%d", i/2), 60)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	credit, err := d.GetOrgCredit(7)
	require.NoError(t, err)
	require.EqualValues(t, -5900, credit.BalancePaise)
	require.False(t, credit.CanStartCall())
	require.Zero(t, credit.MinutesAvailable)
	var entries int
	require.NoError(t, pool.QueryRow(`SELECT COUNT(*) FROM credit_transactions WHERE org_id=7`).Scan(&entries))
	require.Equal(t, workers/2, entries)
	charge, balance, err := d.DeductCallCredits(8, "call-0", 60)
	require.NoError(t, err)
	require.EqualValues(t, 500, charge, "idempotency is scoped to the organization")
	require.EqualValues(t, -400, balance)
	_, err = d.AddCredits(7, 5800, "purchase", "payment-1", "")
	require.NoError(t, err)
	credit, err = d.GetOrgCredit(7)
	require.NoError(t, err)
	require.False(t, credit.CanStartCall(), "a partial recharge does not erase debt")
	_, err = d.AddCredits(7, 101, "purchase", "payment-2", "")
	require.NoError(t, err)
	credit, err = d.GetOrgCredit(7)
	require.NoError(t, err)
	require.EqualValues(t, 1, credit.BalancePaise)
	require.True(t, credit.CanStartCall())
}
