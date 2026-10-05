package db

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestEnsureUserLoginColumns(t *testing.T) {
	t.Run("adds last login column", func(t *testing.T) {
		exec := &migrationExecutor{results: make([]migrationExecResult, 1)}

		require.NoError(t, ensureUserLoginColumns(exec))
		require.Contains(t, exec.queries[0], "ADD COLUMN last_login_at")
	})

	t.Run("accepts existing column", func(t *testing.T) {
		exec := &migrationExecutor{results: []migrationExecResult{{
			err: &mysql.MySQLError{Number: 1060, Message: "Duplicate column name 'last_login_at'"},
		}}}

		require.NoError(t, ensureUserLoginColumns(exec))
	})

	t.Run("returns unexpected database errors", func(t *testing.T) {
		exec := &migrationExecutor{results: []migrationExecResult{{err: errors.New("database unavailable")}}}

		require.ErrorContains(t, ensureUserLoginColumns(exec), "add users.last_login_at")
	})
}

func TestRecordUserLogin(t *testing.T) {
	t.Run("updates the timestamp", func(t *testing.T) {
		exec := &migrationExecutor{results: make([]migrationExecResult, 1)}

		require.NoError(t, recordUserLogin(exec, 42))
		require.Contains(t, exec.queries[0], "last_login_at=UTC_TIMESTAMP()")
		require.Equal(t, []any{int64(42)}, exec.args[0])
	})

	t.Run("returns update errors", func(t *testing.T) {
		exec := &migrationExecutor{results: []migrationExecResult{{err: errors.New("database unavailable")}}}

		require.ErrorContains(t, recordUserLogin(exec, 42), "record user login")
	})
}
