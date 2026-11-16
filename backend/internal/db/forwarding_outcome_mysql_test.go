package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestForwardingOutcomeOrderingMySQL(t *testing.T) {
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
	name := fmt.Sprintf("callified_forward_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE `" + name + "`")
	require.NoError(t, err)
	defer admin.Exec("DROP DATABASE `" + name + "`")
	cfg.DBName = name
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(forwardingOutcomeSchema)
	require.NoError(t, err)
	d := &DB{pool: pool}
	for _, tc := range []struct {
		outcome string
		action  bool
		want    string
	}{
		{"failed", false, "failed"},
		{"busy", true, "busy"},
		{"unknown", false, "busy"},
		{"failed", true, "busy"},
		{"answered", false, "answered"},
		{"cancelled", true, "answered"},
	} {
		require.NoError(t, d.SetForwardingOutcome(context.Background(), 7, "call-1", tc.outcome, "leg-b", tc.action))
		var outcome string
		require.NoError(t, pool.QueryRow("SELECT outcome FROM incoming_forwarding_outcomes WHERE org_id=7 AND call_sid='call-1'").Scan(&outcome))
		require.Equal(t, tc.want, outcome)
	}
	require.NoError(t, d.SetForwardingOutcome(context.Background(), 8, "call-1", "no_answer", "", true))
	var count int
	require.NoError(t, pool.QueryRow("SELECT COUNT(*) FROM incoming_forwarding_outcomes").Scan(&count))
	require.Equal(t, 2, count, "duplicate callbacks are upserts and other organizations remain isolated")
}
