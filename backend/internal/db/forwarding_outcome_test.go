package db

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestForwardingOutcomeUsesAtomicUpsertWithoutChangingInboundStatus(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	for _, tc := range []struct {
		outcome string
		action  bool
	}{{"busy", true}, {"answered", false}, {"no_answer", true}} {
		mock.ExpectExec(`INSERT INTO incoming_forwarding_outcomes .*ON DUPLICATE KEY UPDATE .*WHEN action_received=TRUE THEN outcome`).
			WithArgs(int64(7), "plivo_in_3_call-1", tc.outcome, "leg-b", tc.action).WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, d.SetForwardingOutcome(context.Background(), 7, "plivo_in_3_call-1", tc.outcome, "leg-b", tc.action))
	}
	require.Error(t, d.SetForwardingOutcome(context.Background(), 7, "call-1", "invalid", "", true))
	require.NoError(t, mock.ExpectationsWereMet())
}
